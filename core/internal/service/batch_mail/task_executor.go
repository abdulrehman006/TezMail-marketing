package batch_mail

import (
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/ses_api"
	"billionmail-core/internal/service/warmup"
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gogf/gf/util/grand"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gctx"
	"github.com/panjf2000/ants/v2"
)

var (
	// global task executor map
	taskExecutors      = make(map[int]*TaskExecutor)
	taskExecutorsMutex sync.RWMutex

	// global rate limiter
	//globalLimiter = rate.NewLimiter(rate.Limit(5000), 100)
)

// GetTaskExecutor get task executor
func GetTaskExecutor(taskId int) *TaskExecutor {
	taskExecutorsMutex.RLock()
	defer taskExecutorsMutex.RUnlock()
	return taskExecutors[taskId]
}

// GetOrCreateTaskExecutor get or create task executor
func GetOrCreateTaskExecutor(ctx context.Context, taskId int) *TaskExecutor {
	taskExecutorsMutex.RLock()
	executor, exists := taskExecutors[taskId]
	taskExecutorsMutex.RUnlock()

	if !exists {
		taskExecutorsMutex.Lock()
		defer taskExecutorsMutex.Unlock()
		// double check, avoid race condition
		if executor, exists = taskExecutors[taskId]; !exists {
			executor = NewTaskExecutor(ctx)
			taskExecutors[taskId] = executor
		}
	}
	return executor
}

// RegisterTaskExecutor register task executor
func RegisterTaskExecutor(taskId int, executor *TaskExecutor) {
	taskExecutorsMutex.Lock()
	defer taskExecutorsMutex.Unlock()
	taskExecutors[taskId] = executor
}

// RemoveTaskExecutor remove task executor
func RemoveTaskExecutor(taskId int) {
	taskExecutorsMutex.Lock()
	defer taskExecutorsMutex.Unlock()

	if executor, exists := taskExecutors[taskId]; exists {
		// stop all operations of the executor
		executor.Stop()
		delete(taskExecutors, taskId)
	}
}

// CleanupIdleExecutors cleanup idle executors
func CleanupIdleExecutors() {
	taskExecutorsMutex.Lock()
	defer taskExecutorsMutex.Unlock()

	now := time.Now()
	for id, executor := range taskExecutors {
		if !executor.IsRunning() && now.Sub(executor.lastActivity) > 30*time.Minute {
			executor.Stop()
			delete(taskExecutors, id)
		}
	}
}

// RecoverOrphanedRecipients releases recipients that were claimed for sending
// but whose process died before the outcome was recorded.
//
// getNextRecipientBatch marks a batch is_sent=2 *before* dispatching it. If the
// container restarts mid-batch those rows stay at 2 forever: the work query
// only selects is_sent=0 and the completion check only counts is_sent=1, so the
// task can never reach its total. It stays task_process=1 and the 5s scheduler
// relaunches it every 5 seconds indefinitely, while those recipients are never
// sent. The only existing reset was reachable through PauseTask.
//
// Call this ONCE at startup, before the task scheduler is registered. At that
// point no executor is running in this process, so every is_sent=2 row is by
// definition an orphan from a previous life.
//
// Single-instance assumption: the task scheduler already has no cross-instance
// locking, so two cores against one database would double-send regardless. This
// does not make that worse, but it must not be moved onto a periodic timer,
// where it would race with a live batch and cause duplicate delivery.
func RecoverOrphanedRecipients(ctx context.Context) {
	result, err := g.DB().Model("recipient_info").
		Where("is_sent", 2).
		Data(g.Map{"is_sent": 0}).
		Update()
	if err != nil {
		g.Log().Error(ctx, "failed to recover orphaned recipients on startup:", err)
		return
	}

	if n, _ := result.RowsAffected(); n > 0 {
		g.Log().Warningf(ctx, "recovered %d recipient(s) left mid-send by a previous run; they will be retried", n)
	}
}

// ProcessEmailTasks
func ProcessEmailTasks(ctx context.Context) {
	// get pending tasks
	var tasks []*entity.EmailTask
	err := g.DB().Model("email_tasks").
		Where("task_process IN (0,1)"). // not started or running
		Where("pause", 0). // not paused
		Where("start_time <= ?", time.Now().Unix()). // start time has arrived
		Order("id ASC").
		Scan(&tasks)

	if err != nil {
		g.Log().Error(ctx, "Failed to get pending email tasks: %v", err)
		return
	}

	if len(tasks) == 0 {
		return
	}

	//g.Log().Debug(ctx, "Found %d pending email tasks", len(tasks))

	// process each task
	for _, task := range tasks {
		// check if task already has executor and is running
		executor := GetTaskExecutor(task.Id)
		if executor != nil && executor.IsRunning() {
			continue // skip running task
		}

		// create new executor
		newCtx := gctx.New()
		executor = NewTaskExecutor(newCtx)
		RegisterTaskExecutor(task.Id, executor)

		// start task processing
		go func(taskId int) {
			if err := executor.ProcessTask(newCtx); err != nil {
				g.Log().Error(newCtx, "Error processing task %d: %v", taskId, err)
			}
		}(task.Id)
	}
}

// TaskExecutor task executor
type TaskExecutor struct {
	// context and cancel function
	ctx    context.Context
	cancel context.CancelFunc

	// running status
	isRunning    atomic.Bool
	isPaused     atomic.Bool
	lastActivity time.Time

	// breakerTripped guards the automatic pause so a batch of blocked
	// recipients issues one pause, not one per recipient.
	breakerTripped atomic.Bool

	// task configuration cache
	taskConfig   *entity.EmailTask
	configLoaded time.Time

	spintaxTemplate *SpintaxTemplate

	// rate controller
	rateController *SimpleRateController

	// worker pool
	pool *ants.Pool
	wg   sync.WaitGroup

	// metrics
	sentCount   atomic.Int64
	failedCount atomic.Int64
	startTime   time.Time

	// pause/resume control
	pauseChan  chan struct{}
	resumeChan chan struct{}
}

// SendResult send result
type SendResult struct {
	RecipientID int
	Success     bool
	MessageID   string
	Error       error

	// Retryable says the failure looks transient, so the recipient should go
	// back in the queue rather than being written off. Only meaningful when
	// Success is false.
	Retryable bool

	// AccountBlocked says the whole SES account cannot send -- daily quota
	// exhausted, credentials rejected, account suspended. Every remaining
	// message will fail identically, so the campaign should pause rather than
	// work through the list. The recipient is requeued, not written off.
	AccountBlocked bool

	// AttemptCount is how many attempts this recipient had already had before
	// the one that just finished. Used to decide when to stop retrying.
	AttemptCount int
}

// NewTaskExecutor create task executor
func NewTaskExecutor(ctx context.Context) *TaskExecutor {
	taskCtx, cancel := context.WithCancel(ctx)

	// Set server IP in context
	serverIP, _ := public.GetServerIP()

	taskCtx = context.WithValue(taskCtx, "serverIP", serverIP)

	executor := &TaskExecutor{
		ctx:            taskCtx,
		cancel:         cancel,
		lastActivity:   time.Now(),
		startTime:      time.Now(),
		pauseChan:      make(chan struct{}, 1),
		resumeChan:     make(chan struct{}, 1),
		rateController: NewSimpleRateController(1000),
	}

	return executor
}

// IsRunning check if task is running
func (e *TaskExecutor) IsRunning() bool {
	return e.isRunning.Load()
}

// IsPaused check if task is paused
func (e *TaskExecutor) IsPaused() bool {
	return e.isPaused.Load()
}

// ProcessTask
func (e *TaskExecutor) ProcessTask(ctx context.Context) error {
	// prevent duplicate running
	if !e.isRunning.CompareAndSwap(false, true) {
		return errors.New("task executor is already running")
	}

	defer e.isRunning.Store(false)

	// update activity time
	e.lastActivity = time.Now()

	// get task id
	taskId, err := e.getTaskIdFromContext(ctx)
	if err != nil {
		g.Log().Error(ctx, "failed to get task id: %v", err)
		return err
	}

	// get task info
	task, err := GetTaskInfo(ctx, taskId)
	if err != nil {
		return fmt.Errorf("failed to get task info: %w", err)
	}

	if task == nil || task.Id == 0 {
		return fmt.Errorf("task %d not found", taskId)
	}

	// check if task should run
	if task.TaskProcess == 2 { // completed
		return nil
	}

	currentTaskInfo, err := GetTaskInfo(ctx, taskId)
	if err == nil && currentTaskInfo != nil && currentTaskInfo.TaskProcess == 2 {

		return nil
	}

	// set pause status
	if task.Pause == 1 {
		e.isPaused.Store(true)
	}

	// Pre-flight: refuse to start if the SES account cannot cover this campaign.
	//
	// Without this, a campaign larger than the remaining daily quota starts
	// happily, sends until the allowance runs out, and then produces one 429
	// per remaining recipient. Checking once up front turns thousands of
	// failures into a single actionable message, before any mail is attempted.
	if err := e.checkSendQuotaBeforeStart(ctx, taskId, task); err != nil {
		return err
	}

	// check campaign warmup association and determine warmup identity
	warmupAssociated := false
	var warmupIdentity string

	if warmupStat, _ := warmup.WarmupCampaign().GetWarmupStatusForCampaign(ctx, int64(taskId)); warmupStat != nil {
		warmupAssociated = true

		// Determine warmup identity based on sending method
		// Sending logic prioritizes SES API if configured, so warmup should match:
		// - If SES is configured → use domain-based warmup (SES API will be used for sending)
		// - Else if SMTP is available → use IP-based warmup
		sesAccount := ses_api.GetAccountForDomain(task.Addresser)

		if sesAccount != nil {
			// SES configured → use domain-based warmup (matches sending priority)
			domain := extractDomainFromEmail(task.Addresser)
			if domain != "" {
				warmupIdentity = fmt.Sprintf("ses:%s:%s", sesAccount.Name, domain)
				g.Log().Infof(ctx, "Task %d: Using SES domain-based warmup rate limiting: %s", taskId, warmupIdentity)
			}
		} else if mail_service.IsLocalSMTPEnabled() && mail_service.IsSMTPAvailable(task.Addresser) {
			// No SES, but SMTP available and enabled → IP-based warmup
			warmupIdentity, _ = public.GetServerIP()
			g.Log().Debugf(ctx, "[LOCAL-SMTP-GUARD] Task %d: ALLOWED warmup - local SMTP enabled, using IP-based rate limiting: %s", taskId, warmupIdentity)
		} else {
			g.Log().Debugf(ctx, "[LOCAL-SMTP-GUARD] Task %d: No warmup - local SMTP disabled or SMTP not available for %s", taskId, task.Addresser)
		}
	}

	e.ctx = context.WithValue(e.ctx, "warmupAssociated", warmupAssociated)
	e.ctx = context.WithValue(e.ctx, "warmupIdentity", warmupIdentity)

	// configure rate controller
	e.configureRateController(task)

	// get template info
	template, err := e.getTemplateInfo(ctx, task.TemplateId)
	if err != nil {
		g.Log().Error(ctx, "failed to get template: %v", err)
		return fmt.Errorf("failed to get template: %w", err)
	}

	// process email content
	emailContent := e.processEmailContent(ctx, template.Content, task)

	// create worker pool
	poolSize := task.Threads
	if poolSize <= 0 {
		poolSize = 6
	}

	// detailed record thread parameters
	g.Log().Info(ctx, "task %d: create worker pool, size: %d", task.Id, poolSize)

	// increase worker pool options, improve efficiency
	e.pool, err = ants.NewPool(poolSize,
		ants.WithPreAlloc(true),
		ants.WithPanicHandler(func(p interface{}) {
			g.Log().Error(ctx, "Worker panic: %v", p)
		}),
		ants.WithMaxBlockingTasks(poolSize*100), // allow more waiting tasks
		ants.WithNonblocking(false)) // blocking submit can improve stability

	if err != nil {
		g.Log().Error(ctx, "failed to create worker pool: %v", err)
		return fmt.Errorf("failed to create worker pool: %w", err)
	}

	defer e.pool.Release()

	// update task status to running
	if task.TaskProcess == 0 {
		if err := UpdateTaskProcessStatus(ctx, task.Id, 1); err != nil {
			g.Log().Error(ctx, "failed to update task status: %v", err)
			return fmt.Errorf("failed to update task status: %w", err)
		}
	}

	// start time
	startTime := time.Now()
	//g.Log().Info(ctx, "task %d: start processing, start time: %s", task.Id, startTime.Format("2006-01-02 15:04:05"))

	// process task
	if err := e.processTaskRecipients(ctx, task, emailContent); err != nil {
		g.Log().Error(ctx, "failed to process task: %v", err)
		if errors.Is(err, context.Canceled) {
			g.Log().Info(ctx, "task %d is canceled", task.Id)
			return nil
		}
		return err
	}

	// end time and duration
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	sentCount := e.sentCount.Load()

	// calculate average send rate
	var avgRate float64
	if duration.Seconds() > 0 {
		avgRate = float64(sentCount) / duration.Seconds() * 60
	}

	summaryMsg := fmt.Sprintf("task %d: processing completed, end time: %s, total duration: %.2f minutes, total sent: %d, average rate: %.1f emails/minute",
		task.Id, endTime.Format("2006-01-02 15:04:05"),
		duration.Minutes(), sentCount, avgRate)
	g.Log().Info(ctx, summaryMsg)

	currentTask, err := GetTaskInfo(ctx, taskId)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get current task status: %v", err)
		g.Log().Error(ctx, errMsg)
		return fmt.Errorf("failed to get current task status: %w", err)
	}

	if currentTask.TaskProcess == 2 {
		return nil
	}

	completed, err := e.isTaskComplete(ctx, task.Id)
	if err != nil {
		errMsg := fmt.Sprintf("failed to check completion: %v", err)
		g.Log().Error(ctx, errMsg)
		return fmt.Errorf("failed to check completion: %w", err)
	}

	currentTask, err = GetTaskInfo(ctx, taskId)
	if err == nil && currentTask != nil {
		if currentTask.TaskProcess == 2 {

			return nil
		}
	}

	if completed {
		if err := UpdateTaskProcessStatus(ctx, task.Id, 2); err != nil {
			return fmt.Errorf("failed to update task status: %w", err)
		}
		completeMsg := fmt.Sprintf("task %d is successfully marked as completed", task.Id)
		g.Log().Info(ctx, completeMsg)
		RemoveTaskExecutor(task.Id) // The executor is removed at the end of the task
	}

	return nil
}

// Stop stop task executor
func (e *TaskExecutor) Stop() {
	if e.cancel != nil {
		e.cancel()
	}

	// wait for all work to complete
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	// wait for 3 seconds
	select {
	case <-done:
		// work is completed
	case <-time.After(3 * time.Second):
		// timeout, force stop
	}

	// release worker pool
	if e.pool != nil {
		e.pool.Release()
	}

	e.isRunning.Store(false)
}

// PauseTask
func (e *TaskExecutor) PauseTask(taskId int) error {
	// if already paused, return immediately
	if e.isPaused.Load() {
		return nil
	}

	// set pause status
	e.isPaused.Store(true)

	// Wait for the current batch processing to be completed
	e.waitForCurrentBatch()

	// Reset the records that have been retrieved but not sent (is_sent = 2 -> is_sent = 0)
	resetCount, err := e.resetFetchedRecords(taskId)
	if err != nil {
		e.isPaused.Store(false)
		return fmt.Errorf("failed to reset fetched records: %w", err)
	}

	// update database status
	if err := UpdateTaskPauseStatus(context.Background(), taskId, true); err != nil {
		e.isPaused.Store(false) // restore status
		return fmt.Errorf("failed to update task pause status: %w", err)
	}

	g.Log().Infof(context.Background(), "Task %d paused successfully, reset %d fetched records", taskId, resetCount)
	return nil
}

func (e *TaskExecutor) waitForCurrentBatch() {

	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		g.Log().Debug(context.Background(), "Current batch completed")
	case <-time.After(30 * time.Second):
		g.Log().Warning(context.Background(), "Timeout waiting for current batch to complete")
	}
}

func (e *TaskExecutor) resetFetchedRecords(taskId int) (int64, error) {
	result, err := g.DB().Model("recipient_info").
		Where("task_id", taskId).
		Where("is_sent", 2).
		Data(g.Map{"is_sent": 0}).
		Update()

	if err != nil {
		return 0, err
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

func (e *TaskExecutor) ResumeTask(taskId int) error {
	// if not paused, return immediately
	if !e.isPaused.Load() {
		return nil
	}

	// Reload task configuration (to obtain the latest modifications)
	g.Log().Infof(context.Background(), "Task %d: reloading config before resume...", taskId)
	if err := e.reloadTaskConfig(taskId); err != nil {
		g.Log().Errorf(context.Background(), "Failed to reload task config: %v", err)
		return fmt.Errorf("failed to reload task config: %w", err)
	}

	// restore running status
	e.isPaused.Store(false)

	// Re-arm the circuit breaker. This executor is reused across a
	// pause/resume, so without resetting the breaker a task paused
	// automatically by an account-level block would resume with the breaker
	// already tripped -- and if the account were still blocked, the breaker's
	// trip-once guard would swallow the next block and let the campaign burn a
	// full pass over every remaining recipient before pausing again.
	e.breakerTripped.Store(false)

	// send resume signal
	select {
	case e.resumeChan <- struct{}{}:
		// successfully send resume signal
	default:
		// channel may be full, recreate
		e.resumeChan = make(chan struct{}, 1)
		e.resumeChan <- struct{}{}
	}

	// update database status
	if err := UpdateTaskPauseStatus(context.Background(), taskId, false); err != nil {
		e.isPaused.Store(true) // restore status
		return fmt.Errorf("failed to update task resume status: %w", err)
	}

	g.Log().Info(context.Background(), "Task %d resumed successfully with updated config", taskId)
	return nil
}

// getTaskIdFromContext
//
// Every other access to taskExecutors is guarded by taskExecutorsMutex; this
// one iterated it bare. Registering an executor while another goroutine was
// mid-iteration raised "concurrent map iteration and map write", which is a
// fatal runtime error -- not a panic, so no recover() catches it and the whole
// process dies. The window is widest exactly when several scheduled campaigns
// come due together and are registered in a tight loop.
func (e *TaskExecutor) getTaskIdFromContext(ctx context.Context) (int, error) {
	taskExecutorsMutex.RLock()
	defer taskExecutorsMutex.RUnlock()

	for id, executor := range taskExecutors {
		if executor == e {
			return id, nil
		}
	}
	return 0, errors.New("task id not found in context")
}

// configureRateController
func (e *TaskExecutor) configureRateController(task *entity.EmailTask) {
	maxPerMinute := task.Threads * 20 * 60
	if maxPerMinute <= 0 {
		maxPerMinute = 1000
	}
	g.Log().Info(context.Background(), "task %d: initialize send rate - max %d emails per minute, threads: %d",
		task.Id, maxPerMinute, task.Threads)
	e.rateController = NewSimpleRateController(maxPerMinute)
}

func (e *TaskExecutor) loadTaskConfig(taskId int) error {
	task, err := GetTaskInfo(context.Background(), taskId)
	if err != nil {
		return fmt.Errorf("failed to load task config: %w", err)
	}

	if task == nil || task.Id == 0 {
		return fmt.Errorf("task %d not found", taskId)
	}

	e.taskConfig = task
	e.configLoaded = time.Now()

	g.Log().Infof(context.Background(), "Task %d: config loaded into cache", taskId)
	return nil
}

func (e *TaskExecutor) reloadTaskConfig(taskId int) error {
	g.Log().Infof(context.Background(), "Task %d: reloading config from database...", taskId)
	return e.loadTaskConfig(taskId)
}

func (e *TaskExecutor) getTaskConfig() *entity.EmailTask {
	return e.taskConfig
}

// processTaskRecipients
func (e *TaskExecutor) processTaskRecipients(ctx context.Context, task *entity.EmailTask, emailContent string) error {
	const batchSize = 50
	var lastId = 0

	// add performance monitoring timer
	statsTicker := time.NewTicker(15 * time.Second)
	defer statsTicker.Stop()

	// record last check time and sent count
	lastCheckTime := time.Now()
	lastSentCount := int64(0)

	// start statistics goroutine
	go func() {
		for {
			select {
			case <-statsTicker.C:
				currentTime := time.Now()
				currentSent := e.sentCount.Load()

				// calculate interval send rate
				elapsedSeconds := currentTime.Sub(lastCheckTime).Seconds()
				sentInInterval := currentSent - lastSentCount
				ratePerMinute := float64(0)
				if elapsedSeconds > 0 {
					ratePerMinute = float64(sentInInterval) / elapsedSeconds * 60
				}

				// get task id
				taskId, _ := e.getTaskIdFromContext(ctx)

				infoMsg := fmt.Sprintf("task %d: performance stats - %.1f seconds sent %d emails, rate: %.1f emails/minute, goroutine pool usage: %d/%d",
					taskId, elapsedSeconds, sentInInterval, ratePerMinute,
					e.pool.Running(), e.pool.Cap())
				g.Log().Info(ctx, infoMsg)

				// update baseline value
				lastCheckTime = currentTime
				lastSentCount = currentSent

			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		// check if context is canceled
		select {
		case <-ctx.Done():
			g.Log().Info(ctx, "context canceled, stop task execution:", ctx.Err())
			return ctx.Err()
		default:
		}

		// check pause status
		if e.isPaused.Load() {
			g.Log().Debug(ctx, "Task %d is paused, waiting for resume signal", task.Id)

			// wait for resume signal
			select {
			case <-e.resumeChan:

				if e.taskConfig != nil {
					g.Log().Infof(ctx, "Using updated task config after resume: addresser=%s, subject=%s, template_id=%d, full_name=%s",
						e.taskConfig.Addresser, e.taskConfig.Subject, e.taskConfig.TemplateId, e.taskConfig.FullName)

					task = e.taskConfig

					template, err := e.getTemplateInfo(ctx, task.TemplateId)
					if err != nil {
						g.Log().Errorf(ctx, "failed to get updated template: %v", err)
					} else {
						emailContent = e.processEmailContent(ctx, template.Content, task)

					}
				}
			case <-ctx.Done():
				g.Log().Info(ctx, "context canceled, stop task execution:", ctx.Err())
				return ctx.Err()
			}
		}

		// get a batch of recipients to send
		recipients, err := e.getNextRecipientBatch(ctx, task.Id, lastId, batchSize)
		if err != nil {
			return fmt.Errorf("failed to get recipients: %w", err)
		}

		// no more recipients, exit loop
		if len(recipients) == 0 {
			break
		}

		// record batch size
		//g.Log().Debug(ctx, "task %d: got %d recipients to send", task.Id, len(recipients))

		// update last id
		lastId = recipients[len(recipients)-1].Id

		// process this batch of recipients
		if err := e.processRecipientBatch(ctx, task, recipients, emailContent); err != nil {
			return err
		}

		// adjust send rate
		e.rateController.AdjustRate()
	}

	// wait for all tasks to complete
	//g.Log().Info(ctx, "task %d: all recipients processed, waiting for remaining send tasks to complete...", task.Id)
	e.wg.Wait()
	g.Log().Info(ctx, "task %d: all send tasks completed", task.Id)
	return nil
}

// getNextRecipientBatch
func (e *TaskExecutor) getNextRecipientBatch(ctx context.Context, taskId, lastId, batchSize int) ([]*entity.RecipientInfo, error) {
	var recipients []*entity.RecipientInfo

	err := g.DB().Model("recipient_info").
		Where("task_id", taskId).
		Where("is_sent", 0).
		Where("sent_time <= ?", time.Now().Unix()).
		Where("id > ?", lastId).
		Order("id ASC").
		Limit(batchSize).
		Scan(&recipients)

	if err != nil || len(recipients) == 0 {
		return recipients, err
	}

	ids := make([]int, len(recipients))
	for i, r := range recipients {
		ids[i] = r.Id
	}

	_, err = g.DB().Model("recipient_info").
		WhereIn("id", ids).
		Data(g.Map{"is_sent": 2}).
		Update()

	if err != nil {
		g.Log().Error(ctx, "Failed to mark recipients as fetched: %v", err)
		return nil, err
	}

	return recipients, nil
}

// processRecipientBatch
func (e *TaskExecutor) processRecipientBatch(ctx context.Context, task *entity.EmailTask, recipients []*entity.RecipientInfo, emailContent string) error {
	// create result channel, buffer size same as recipient count
	resultChan := make(chan *SendResult, len(recipients))

	// create wait group to track all send tasks
	var sendWg sync.WaitGroup

	// create a mutex and flag to control channel closure
	var mu sync.Mutex
	channelClosed := false

	// create a safe send function
	safeSend := func(result *SendResult) {
		mu.Lock()
		defer mu.Unlock()
		if !channelClosed {
			select {
			case resultChan <- result:
				// successfully sent
			case <-ctx.Done():
				// context canceled, no more send
			}
		}
	}

	// safe close channel function
	safeClose := func() {
		mu.Lock()
		defer mu.Unlock()
		if !channelClosed {
			channelClosed = true
			close(resultChan)
		}
	}

	updates := make(map[int]int)

	// submit send task for each recipient
	for _, recipient := range recipients {
		// check again if paused or canceled
		if e.isPaused.Load() {
			select {
			case <-e.resumeChan:
				// resumed
			case <-ctx.Done():
				safeClose() // safe close channel
				return ctx.Err()
			}
		}

		select {
		case <-ctx.Done():
			safeClose()
			return ctx.Err()
		default:
		}

		// wait for rate control
		if err := e.rateController.Wait(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				safeClose() // safe close channel
				return err
			}
			// record error but continue
			g.Log().Debugf(ctx, "Rate limit wait error: %v", err)

		}

		// check if recipient is allowed to send with warmup
		// This enforces warmup-compliant spacing between emails (only when warmup is enabled)
		if warmupAssociated, ok := e.ctx.Value("warmupAssociated").(bool); ok && warmupAssociated {
			// Determine warmup identity based on sending method
			var warmupIdentity string
			if identity, ok := e.ctx.Value("warmupIdentity").(string); ok && identity != "" {
				warmupIdentity = identity
			} else {
				// Fallback to server IP (legacy behavior)
				warmupIdentity, _ = e.ctx.Value("serverIP").(string)
			}

			if warmupIdentity != "" {
				allow, waitSeconds, _ := warmup.RateLimiter().Allow(ctx, warmupIdentity, recipient.Recipient)

				if !allow || waitSeconds > 0 {
					// Always defer - never sleep inline (production-grade approach)
					// Daily count is NOT incremented for deferred attempts
					retryAfter := waitSeconds
					if retryAfter < 60 {
						retryAfter = 60 // Minimum 60 seconds (Gmail-safe during warm-up)
					}
					updates[recipient.Id] = retryAfter
					g.Log().Debug(ctx, "Warmup: recipient %d deferred, retry after %d seconds", recipient.Id, retryAfter)
					continue
				}
			}
		}

		// create recipient copy to avoid closure problem
		recipientBak := recipient

		// add wait count
		e.wg.Add(1)
		sendWg.Add(1)

		// submit to worker pool
		err := e.pool.Submit(func() {
			defer e.wg.Done()
			defer sendWg.Done()

			// Guarantee exactly one result reaches the collector.
			//
			// safeSend used to be the last statement in this function, so a
			// panic anywhere above it delivered nothing. The pool's panic
			// handler kept the process alive and the WaitGroups still fired,
			// but the recipient was left at is_sent=2 -- invisible to both the
			// work query and the completion count, so the task could never
			// finish and the scheduler relaunched it every 5 seconds forever.
			//
			// Delivering from a defer means the result is sent whether this
			// function returns normally or unwinds through a panic.
			var result *SendResult
			defer func() {
				if r := recover(); r != nil {
					g.Log().Errorf(ctx, "recovered panic sending to recipient %d: %v\n%s",
						recipientBak.Id, r, debug.Stack())
					result = nil // fall through to the safety net below
				}
				if result == nil {
					// Retryable: a panic says something is wrong with our code
					// or this particular message, not with the recipient, and
					// the attempt cap bounds any repeat.
					result = &SendResult{
						RecipientID:  recipientBak.Id,
						Success:      false,
						Error:        fmt.Errorf("send aborted unexpectedly"),
						Retryable:    true,
						AttemptCount: recipientBak.AttemptCount,
					}
					e.failedCount.Add(1)
				}
				safeSend(result)
			}()
			// print task id
			//g.Log().Debug(ctx, "current task id", task.Id, "sender-", task.Addresser, "recipient-", recipientBak.Recipient)
			// personalize content
			personalized, _, _ := e.personalizeEmail(ctx, emailContent, task, recipientBak)
			//personalized := emailContent

			// send email
			// Assigns to the outer `result`, which the deferred block above
			// delivers. Do not shadow it with := or the panic-safety net will
			// send a placeholder instead of the real outcome.
			result = e.sendEmail(ctx, task, recipientBak, personalized)

			// use sendEmailMock (Don't use in production)
			// result = e.sendEmailMock(ctx, task, recipientBak, personalized)

			// record send
			e.rateController.RecordSend()

			// update stats
			if result != nil && result.Success {
				e.sentCount.Add(1)
			} else {
				e.failedCount.Add(1)
			}
		})

		if err != nil {
			e.wg.Done()   // reduce wait count
			sendWg.Done() // reduce send wait count

			// create failed result
			// Pool saturation or shutdown says nothing about the recipient, so
			// requeue rather than writing them off.
			failResult := &SendResult{
				RecipientID:  recipient.Id,
				Success:      false,
				Error:        fmt.Errorf("failed to submit to worker pool: %w", err),
				Retryable:    true,
				AttemptCount: recipient.AttemptCount,
			}

			// safe send result
			safeSend(failResult)
		}
	}

	if len(updates) > 0 {
		curTime := int(time.Now().Unix())
		i := 0
		for id, waits := range updates {
			// Simple stagger: waits + i ensures unique sent_time for each recipient
			// - waits: minimum wait from rate limiter (typically 60 sec)
			// - i: adds 1 second per recipient to prevent duplicates
			// This works for both small (20) and large (200K) campaigns:
			// - Rate limiter controls actual sending speed
			// - Unique sent_times prevent batch overload
			newSentTime := curTime + waits + i
			// Reset is_sent=0 and update sent_time for deferred recipients
			_, _ = g.DB().Ctx(ctx).Model("recipient_info").
				Where("id", id).
				Data(g.Map{
					"is_sent":   0,
					"sent_time": newSentTime,
				}).
				Update()
			i++
		}
	}

	// all tasks submitted, start result processing and channel closure goroutine
	resultsDone := make(chan struct{})

	// start goroutine to close channel
	go func() {
		// wait for all send tasks to complete or context canceled
		sendDone := make(chan struct{})
		go func() {
			sendWg.Wait()
			close(sendDone)
		}()

		select {
		case <-sendDone:
			// all send tasks completed, safe close channel
			safeClose()
		case <-ctx.Done():
			// context canceled, safe close channel
			safeClose()
		}
	}()

	// start result processing goroutine
	go func() {
		e.processSendResults(ctx, resultChan)
		close(resultsDone)
	}()

	// wait for result processing to complete or context canceled
	select {
	case <-resultsDone:
		// result processing completed
		return nil
	case <-ctx.Done():
		// context canceled
		return ctx.Err()
	}
}

// processSendResults
func (e *TaskExecutor) processSendResults(ctx context.Context, resultChan <-chan *SendResult) {
	const batchSize = 50
	const flushInterval = 200 * time.Millisecond

	successResults := make([]*SendResult, 0, batchSize)
	retryFailures := make([]*SendResult, 0, batchSize)
	terminalFailures := make([]*SendResult, 0, batchSize)
	blockedFailures := make([]*SendResult, 0, batchSize)

	// create ticker to flush results
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	// flush function
	flushUpdates := func() {
		if len(successResults) == 0 && len(retryFailures) == 0 &&
			len(terminalFailures) == 0 && len(blockedFailures) == 0 {
			return
		}

		// process success records
		if len(successResults) > 0 {
			// prepare batch update
			now := time.Now().Unix()
			e.lastActivity = time.Now()
			// batch update recipient status
			// prepare batch update SQL
			if len(successResults) > 0 {
				// method 2: use SQL batch update
				ids := make([]interface{}, 0, len(successResults))
				messageIds := make(map[int]string, len(successResults))

				for _, result := range successResults {
					ids = append(ids, result.RecipientID)
					messageIds[result.RecipientID] = result.MessageID
				}

				// step 1: batch update is sent and sent time
				_, err := g.DB().Model("recipient_info").
					WhereIn("id", ids).
					Data(g.Map{
						"is_sent":   1,
						"sent_time": now,
					}).
					Update()

				if err != nil {
					g.Log().Error(ctx, "batch update recipient status failed: %v", err)
				} else {
					// step 2: update each recipient's message ID
					for id, messageID := range messageIds {
						// remove message_id external < >
						messageID = strings.Trim(messageID, "<>")
						_, err := g.DB().Model("recipient_info").
							Where("id", id).
							Data(g.Map{"message_id": messageID}).
							Update()

						if err != nil {
							g.Log().Error(ctx, "update recipient(ID:%d) message ID failed: %v", id, err)
						}
					}

					g.Log().Debug(ctx, "successfully batch updated %d recipient status", len(successResults))
				}
			}

			// clear success results
			successResults = successResults[:0]
		}

		// Failures split two ways.
		//
		// Previously every failure was written is_sent=1 -- identical to a
		// delivered recipient -- with a comment explaining it was done to stop
		// the task getting stuck. That bought liveness at the cost of silently
		// discarding mail: the row could never be selected again, and nothing
		// recorded that it had failed or why. A brief SES blip during a large
		// campaign dropped every recipient in flight while the task reported
		// 100% complete.
		//
		// Transient failures now go back to is_sent=0 with sent_time pushed
		// into the future, which is the same deferral mechanism warmup already
		// uses -- getNextRecipientBatch selects on is_sent=0 AND sent_time <=
		// now, so the row is simply picked up on a later pass. Permanent
		// failures, and transients that have exhausted their attempts, still go
		// terminal at is_sent=1 so the task can finish, but are now flagged
		// send_failed=1 with the reason recorded.
		//
		// is_sent keeps its exact meaning, so every existing query that counts
		// is_sent=1 for progress or completion returns what it always did.
		// Grouped rather than per-row. Failures are rare in normal operation, but
		// during an outage every recipient in the batch fails at once -- and
		// they nearly always share the same attempt count and the same error, so
		// grouping collapses what would be one UPDATE per recipient into
		// typically a single statement. That keeps the failure path from adding
		// database load exactly when the system is already struggling.
		if len(retryFailures) > 0 {
			now := time.Now().Unix()
			for key, ids := range groupFailures(retryFailures) {
				_, err := g.DB().Model("recipient_info").
					WhereIn("id", ids).
					Data(g.Map{
						"is_sent":       0,
						"sent_time":     now + retryBackoffSeconds(key.attempt),
						"attempt_count": key.attempt + 1,
						"last_error":    key.errText,
					}).
					Update()
				if err != nil {
					g.Log().Error(ctx, "failed to requeue recipients for retry: %v", err)
				}
			}
			g.Log().Infof(ctx, "requeued %d recipient(s) after a transient failure", len(retryFailures))
			retryFailures = retryFailures[:0]
		}

		if len(terminalFailures) > 0 {
			now := time.Now().Unix()
			for key, ids := range groupFailures(terminalFailures) {
				_, err := g.DB().Model("recipient_info").
					WhereIn("id", ids).
					Data(g.Map{
						"is_sent":       1,
						"sent_time":     now,
						"send_failed":   1,
						"attempt_count": key.attempt + 1,
						"last_error":    key.errText,
					}).
					Update()
				if err != nil {
					g.Log().Error(ctx, "failed to record terminal failures: %v", err)
				}
			}
			g.Log().Warningf(ctx, "%d recipient(s) permanently failed and will not be retried", len(terminalFailures))
			terminalFailures = terminalFailures[:0]
		}

		// Account-level block: the whole account cannot send, so every
		// remaining message would fail identically.
		//
		// These recipients go straight back to pending WITHOUT consuming an
		// attempt. Nothing is wrong with them -- the account is unavailable --
		// so charging them a retry would mean a quota exhaustion silently ate a
		// third of every recipient's budget. sent_time is left alone so they
		// are immediately eligible when the campaign resumes.
		if len(blockedFailures) > 0 {
			for key, ids := range groupFailures(blockedFailures) {
				_, err := g.DB().Model("recipient_info").
					WhereIn("id", ids).
					Data(g.Map{
						"is_sent":    0,
						"last_error": key.errText,
					}).
					Update()
				if err != nil {
					g.Log().Error(ctx, "failed to requeue recipients after an account-level block: %v", err)
				}
			}
			g.Log().Warningf(ctx, "%d recipient(s) requeued because the SES account is unavailable", len(blockedFailures))

			// Trip the breaker once, using the first failure's reason.
			e.tripCircuitBreaker(ctx, blockedFailures[0].Error)
			blockedFailures = blockedFailures[:0]
		}
	}

	// main loop
	for {
		select {
		case result, ok := <-resultChan:
			if !ok {
				// channel closed, process remaining results
				flushUpdates()
				return
			}

			if result.Success {
				successResults = append(successResults, result)
			} else if result.AccountBlocked {
				// Checked before the retry branch: an account-level block must
				// never be treated as an ordinary retryable failure, or it
				// would consume attempts and eventually write recipients off
				// for a condition that has nothing to do with them.
				blockedFailures = append(blockedFailures, result)
				g.Log().Warningf(ctx, "send to recipient %d blocked at account level: %v",
					result.RecipientID, result.Error)
			} else if result.Retryable && result.AttemptCount+1 < maxSendAttempts {
				retryFailures = append(retryFailures, result)
				g.Log().Debugf(ctx, "send to recipient %d failed (attempt %d/%d, will retry): %v",
					result.RecipientID, result.AttemptCount+1, maxSendAttempts, result.Error)
			} else {
				terminalFailures = append(terminalFailures, result)
				g.Log().Debugf(ctx, "send to recipient %d failed permanently after %d attempt(s): %v",
					result.RecipientID, result.AttemptCount+1, result.Error)
			}

			// reach batch processing size, flush
			if len(successResults)+len(retryFailures)+len(terminalFailures)+len(blockedFailures) >= batchSize {
				flushUpdates()
			}

		case <-ticker.C:
			// flush periodically
			flushUpdates()

		case <-ctx.Done():
			// context canceled, process remaining results
			flushUpdates()
			return
		}
	}
}

// getTemplateInfo get template info
func (e *TaskExecutor) getTemplateInfo(ctx context.Context, templateId int) (*entity.EmailTemplate, error) {
	var template entity.EmailTemplate

	err := g.DB().Model("email_templates").
		Where("id", templateId).
		Scan(&template)

	if err != nil {
		return nil, err
	}

	if template.Id == 0 {
		return nil, fmt.Errorf("template %d not found", templateId)
	}

	return &template, nil
}

// processEmailContent
func (e *TaskExecutor) processEmailContent(ctx context.Context, content string, task *entity.EmailTask) string {
	// process unsubscribe link
	if task.Unsubscribe == 1 {
		// __UNSUBSCRIBE_URL__  {{ .UnsubscribeURL }}
		if !strings.Contains(content, "__UNSUBSCRIBE_URL__") && !strings.Contains(content, "{{ .UnsubscribeURL }}") {
			content = public.AddUnsubscribeButton(content)
		}

		content = strings.ReplaceAll(content, "__UNSUBSCRIBE_URL__", "{{ .UnsubscribeURL }}")
	}

	// Preparse the spintax template
	if e.spintaxTemplate == nil {
		spintaxParser := GetSpintaxParser()
		e.spintaxTemplate = spintaxParser.ParseTemplate(content)
	}

	return content
}

// personalizeEmail personalize email content
// Returns: renderedContent, renderedSubject, unsubscribeURL
func (e *TaskExecutor) personalizeEmail(ctx context.Context, content string, task *entity.EmailTask, recipient *entity.RecipientInfo) (string, string, string) {

	var contact entity.Contact
	// 优先按任务分组精确匹配，再按创建时间倒序获取最新一条
	q := g.DB().Model("bm_contacts").Where("email", recipient.Recipient)
	if task.GroupId > 0 {
		q = q.Where("group_id", task.GroupId)
	}
	if err := q.OrderDesc("create_time").Limit(1).Scan(&contact); err != nil {
		g.Log().Error(ctx, "get contact info failed: %v", err)
	}

	// If no records are found or the grouping does not match, revert to retrieving only the latest record based on the email address.
	if contact.Id == 0 {
		if err := g.DB().Model("bm_contacts").Where("email", recipient.Recipient).OrderDesc("create_time").Limit(1).Scan(&contact); err != nil {
			g.Log().Error(ctx, "fallback get contact by email failed: %v", err)
		}
	}

	var emailtask entity.EmailTask
	err := g.DB().Model("email_tasks").Where("id", task.Id).Scan(&emailtask)
	if err != nil {
		g.Log().Error(ctx, "get task info failed: %v", err)
		emailtask = *task
	}

	// Unsubscribe
	var renderedContent, renderedSubject string
	var unsubscribeJumpURL string
	engine := GetTemplateEngine()

	if task.Unsubscribe == 1 {
		//domain := domains.GetBaseURLBySender(task.Addresser)
		domain := domains.GetBaseURL()

		var contactGroupId int
		contactGroupId = task.GroupId

		jwtToken, err := GenerateUnsubscribeJWT(
			recipient.Recipient,
			task.TemplateId,
			task.Id,
			contactGroupId,
		)
		if err != nil {
			g.Log().Error(ctx, "generate unsubscribe JWT failed: %v", err)
			jwtToken = ""
		}

		if contactGroupId > 0 {

			unsubscribeJumpURL = fmt.Sprintf("%s/unsubscribe_new.html?jwt=%s",
				domain, jwtToken)

		} else {

			unsubscribeURL := fmt.Sprintf("%s/api/unsubscribe", domain)
			groupURL := fmt.Sprintf("%s/api/unsubscribe/user_group", domain)
			unsubscribeJumpURL = fmt.Sprintf("%s/unsubscribe.html?jwt=%s&email=%s&url_type=%s&url_unsubscribe=%s",
				domain, jwtToken, recipient.Recipient, groupURL, unsubscribeURL)
		}

		// render email content
		renderedContent, err = engine.RenderEmailTemplate(ctx, content, &contact, &emailtask, unsubscribeJumpURL)
		if err != nil {
			g.Log().Error(ctx, "render email content failed: %v", err)
			renderedContent = content
		}

		// render email subject
		renderedSubject, err = engine.RenderEmailTemplate(ctx, emailtask.Subject, &contact, &emailtask, unsubscribeJumpURL)
		if err != nil {
			g.Log().Error(ctx, "render email subject failed: %v", err)
			renderedSubject = emailtask.Subject
		}
	} else {
		// if unsubscribe is not enabled, render email content
		renderedContent, err = engine.RenderEmailTemplate(ctx, content, &contact, &emailtask, "")
		if err != nil {
			g.Log().Error(ctx, "render email content failed: %v", err)
			renderedContent = content
		}

		// render email subject
		renderedSubject, err = engine.RenderEmailTemplate(ctx, emailtask.Subject, &contact, &emailtask, "")
		if err != nil {
			g.Log().Error(ctx, "render email subject failed: %v", err)
			renderedSubject = emailtask.Subject
		}
	}

	// Restore the erroneous variable
	renderedContent = e.restoreErrorVariables(renderedContent)
	renderedSubject = e.restoreErrorVariables(renderedSubject)

	return renderedContent, renderedSubject, unsubscribeJumpURL
}

// restoreErrorVariables 恢复 [__变量__] 为 {{变量}}
func (e *TaskExecutor) restoreErrorVariables(content string) string {
	re := regexp.MustCompile(`\[__([^_]+)__\]`)
	return re.ReplaceAllString(content, "{{$1}}")
}

// sendEmail send email
func (e *TaskExecutor) sendEmail(ctx context.Context, task *entity.EmailTask, recipient *entity.RecipientInfo, content string) *SendResult {
	// check if context is canceled
	select {
	case <-ctx.Done():
		// The campaign was paused or stopped before this recipient was ever
		// attempted. Marking them terminal here discarded their mail outright,
		// so requeue instead -- nothing about the recipient failed.
		return &SendResult{
			RecipientID:  recipient.Id,
			Success:      false,
			Error:        ctx.Err(),
			Retryable:    true,
			AttemptCount: recipient.AttemptCount,
		}
	default:
		// continue execution
	}

	currentTask := task
	if e.taskConfig != nil {
		currentTask = e.taskConfig
	}

	// get rendered content, subject and unsubscribe URL
	renderedContent, renderedSubject, unsubscribeURL := e.personalizeEmail(ctx, content, currentTask, recipient)

	// Generate message ID first (needed for both SMTP and SES)
	messageID := generateMessageID(currentTask.Addresser)

	//Tracking emails
	baseURL := domains.GetBaseURL()
	mail_tracker := maillog_stat.NewMailTracker(renderedContent, currentTask.Id, messageID, recipient.Recipient, baseURL)
	mail_tracker.TrackLinks()
	mail_tracker.AppendTrackingPixel()
	renderedContent = mail_tracker.GetHTML()

	// Try SES API first if configured for this domain
	sesAccount := ses_api.GetAccountForDomain(currentTask.Addresser)
	if sesAccount != nil {
		return e.sendEmailViaSESApi(ctx, currentTask, recipient, renderedContent, renderedSubject, unsubscribeURL, messageID)
	}

	// Fall back to SMTP if enabled
	if !mail_service.IsLocalSMTPEnabled() {
		g.Log().Warning(ctx, "[LOCAL-SMTP-GUARD] task_executor.sendEmail: BLOCKED - Local SMTP disabled, no SES for", currentTask.Addresser)
		// RecipientID was omitted here, defaulting to 0. That zero went into
		// failedIDs, so the batch update targeted row id=0 and matched nothing,
		// leaving the real recipient stuck at is_sent=2 -- invisible to both the
		// is_sent=0 work query and the is_sent=1 completion count, so the task
		// could never finish and was relaunched every 5s forever.
		return &SendResult{RecipientID: recipient.Id, Success: false, Error: fmt.Errorf("local SMTP is disabled and no SES configured for this domain")}
	}
	g.Log().Info(ctx, "[LOCAL-SMTP-GUARD] task_executor.sendEmail: ALLOWED - falling back to SMTP for", currentTask.Addresser)
	return e.sendEmailViaSMTP(ctx, currentTask, recipient, renderedContent, renderedSubject, unsubscribeURL, messageID)
}

// sendEmailViaSESApi sends email using Amazon SES API
func (e *TaskExecutor) sendEmailViaSESApi(ctx context.Context, task *entity.EmailTask, recipient *entity.RecipientInfo, content, subject, unsubscribeURL, messageID string) *SendResult {
	sesSender, accountName, err := ses_api.GetSenderForEmail(ctx, task.Addresser)
	if err != nil {
		g.Log().Error(ctx, "Failed to create SES sender for", task.Addresser, ":", err)
		// Fall back to SMTP if enabled
		if !mail_service.IsLocalSMTPEnabled() {
			g.Log().Warning(ctx, "[LOCAL-SMTP-GUARD] task_executor.sendEmailViaSESApi: BLOCKED - SES sender failed, local SMTP disabled for", task.Addresser)
			// See the note above: a missing RecipientID stranded the recipient
			// at is_sent=2 and wedged the task permanently.
			return &SendResult{RecipientID: recipient.Id, Success: false, Error: fmt.Errorf("SES failed and local SMTP is disabled")}
		}
		g.Log().Info(ctx, "[LOCAL-SMTP-GUARD] task_executor.sendEmailViaSESApi: ALLOWED - SES sender failed, falling back to SMTP for", task.Addresser)
		return e.sendEmailViaSMTP(ctx, task, recipient, content, subject, unsubscribeURL, messageID)
	}

	// Build From address with display name.
	// RFC 5322 3.4 quoting/encoding, so a name containing a comma or non-ASCII
	// characters does not fail the whole campaign at SES validation.
	fromAddress := ses_api.FormatFromAddress(task.FullName, task.Addresser)

	// Build custom headers
	headers := make(map[string]string)
	if unsubscribeURL != "" {
		headers["List-Unsubscribe"] = fmt.Sprintf("<%s>", unsubscribeURL)
		headers["List-Unsubscribe-Post"] = "List-Unsubscribe=One-Click"
	}

	// Send via SES API
	input := &ses_api.SendEmailInput{
		From:      fromAddress,
		To:        []string{recipient.Recipient},
		Subject:   subject,
		HtmlBody:  content,
		Headers:   headers,
		MessageID: messageID,
	}

	result := sesSender.SendEmail(ctx, input)

	if !result.Success {
		g.Log().Error(ctx, "SES API send failed for", recipient.Recipient, ":", result.Error)

		// Try SMTP fallback if enabled.
		//
		// The stats row used to be written here, before the fallback ran, so a
		// message Postfix went on to deliver successfully still recorded a
		// 'bounced' row. That both inflated the bounce rate and destroyed the
		// only record that could tell "lost" apart from "delivered via
		// fallback" -- the two produced identical rows. It is now written only
		// on the paths where the message really was not delivered.
		if mail_service.IsLocalSMTPEnabled() {
			g.Log().Info(ctx, "[LOCAL-SMTP-GUARD] task_executor.sendEmailViaSESApi: ALLOWED - SES send failed, falling back to SMTP for", recipient.Recipient)
			fallback := e.sendEmailViaSMTP(ctx, task, recipient, content, subject, unsubscribeURL, messageID)
			if fallback != nil && fallback.Success {
				// Delivered over Postfix. Its own log parser records the send,
				// so writing a synthetic bounce here would double-count it.
				g.Log().Warning(ctx, "[SES] message to", recipient.Recipient, "was delivered via local SMTP after SES failed. SPF/DKIM are aligned to SES for this domain, so this message may fail DMARC at the receiver.")
				return fallback
			}
			// Fallback failed too: the recipient really is lost, so record it.
			e.recordSESFailureStats(ctx, task, recipient, accountName, messageID, len(content), result.Error)
			return fallback
		}

		g.Log().Warning(ctx, "[LOCAL-SMTP-GUARD] task_executor.sendEmailViaSESApi: BLOCKED - SES send failed, local SMTP disabled, no fallback for", recipient.Recipient)
		e.recordSESFailureStats(ctx, task, recipient, accountName, messageID, len(content), result.Error)

		return &SendResult{
			RecipientID:    recipient.Id,
			Success:        false,
			Error:          result.Error,
			Retryable:      ses_api.IsRetryable(result.Error),
			AccountBlocked: ses_api.IsAccountBlocked(result.Error),
			AttemptCount:   recipient.AttemptCount,
		}
	}

	g.Log().Debug(ctx, "Email sent via SES API (account:", accountName, ") to:", recipient.Recipient)

	// Record the SES-sent email in mailstat tables for statistics tracking
	// Since SES API bypasses Postfix, we need to manually insert the stats
	safeGo(ctx, "SES success stats writer", func() {
		nowMillis := time.Now().UnixMilli()
		cleanMessageID := strings.Trim(messageID, "<>")

		// Insert into mailstat_message_ids to map message_id
		// Use SES message ID as postfix_message_id since there's no Postfix involved
		sesPostfixID := sesStatsKey("ses-", result.MessageID, cleanMessageID)
		_, err := g.DB().Model("mailstat_message_ids").InsertIgnore(g.Map{
			"postfix_message_id": sesPostfixID,
			"message_id":         cleanMessageID,
			"log_time_millis":    nowMillis,
		})
		if err != nil {
			g.Log().Warning(ctx, "Failed to insert SES message ID mapping:", err)
		}

		// Insert into mailstat_senders to record sender info
		_, err = g.DB().Model("mailstat_senders").InsertIgnore(g.Map{
			"postfix_message_id": sesPostfixID,
			"sender":             task.Addresser,
			"size":               len(content),
			"log_time_millis":    nowMillis,
		})
		if err != nil {
			g.Log().Warning(ctx, "Failed to insert SES sender stats:", err)
		}

		// Insert into mailstat_send_mails with status 'sent'
		_, err = g.DB().Model("mailstat_send_mails").InsertIgnore(g.Map{
			"postfix_message_id": sesPostfixID,
			"recipient":          recipient.Recipient,
			"mail_provider":      public.GetMailProviderGroup(recipient.Recipient),
			"status":             "sent",
			"delay":              0,
			"delays":             "0/0/0/0",
			"dsn":                "2.0.0",
			"relay":              "ses-api[" + accountName + "]",
			"description":        "Delivered via Amazon SES API",
			"log_time_millis":    nowMillis,
		})
		if err != nil {
			g.Log().Warning(ctx, "Failed to insert SES send stats:", err)
		}
	})

	// Return the original messageID we generated (not the SES message ID)
	// This ensures recipient_info.message_id matches mailstat_message_ids.message_id
	return &SendResult{
		RecipientID: recipient.Id,
		MessageID:   messageID,
		Success:     true,
		Error:       nil,
	}
}

// sendEmailViaSMTP sends email using traditional SMTP (existing behavior)
func (e *TaskExecutor) sendEmailViaSMTP(ctx context.Context, task *entity.EmailTask, recipient *entity.RecipientInfo, content, subject, unsubscribeURL, messageID string) *SendResult {
	sender, err := mail_service.NewEmailSenderWithLocal(task.Addresser)
	if err != nil {
		g.Log().Error(ctx, "create email sender failed: %v", err)
		// Could be a missing mailbox (permanent) or a transient database or
		// connection problem. Retry rather than discard -- the attempt cap
		// bounds the cost of guessing wrong, whereas discarding loses the mail.
		return &SendResult{
			RecipientID:  recipient.Id,
			Success:      false,
			Error:        fmt.Errorf("create email sender failed: %w", err),
			Retryable:    true,
			AttemptCount: recipient.AttemptCount,
		}
	}
	defer sender.Close()

	// create email message with rendered subject
	message := mail_service.NewMessage(subject, content)
	message.SetMessageID(messageID)

	// set sender display name
	if task.FullName != "" {
		message.SetRealName(task.FullName)
	}

	// Add List-Unsubscribe header for better deliverability (RFC 2369)
	if unsubscribeURL != "" {
		message.SetHeader("List-Unsubscribe", fmt.Sprintf("<%s>", unsubscribeURL))
		message.SetHeader("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}

	// send email
	err = sender.Send(message, []string{recipient.Recipient})
	if err != nil {
		g.Log().Error(ctx, "send email to %s failed: %v", recipient.Recipient, err)
		// SMTP delivery problems are usually connection-level and clear on
		// their own, so this is worth another attempt.
		return &SendResult{
			RecipientID:  recipient.Id,
			Success:      false,
			Error:        fmt.Errorf("send email failed: %w", err),
			Retryable:    true,
			AttemptCount: recipient.AttemptCount,
		}
	}

	return &SendResult{
		RecipientID: recipient.Id,
		MessageID:   messageID,
		Success:     true,
		Error:       nil,
	}
}

// tripCircuitBreaker pauses the task because the SES account cannot send.
//
// Without this, an account-level condition -- exhausted daily quota, rejected
// credentials, suspended account -- would let the campaign walk the entire
// remaining list producing one identical error per recipient. Pausing turns
// that into a single stop with a reason an operator can act on, and leaves
// every unsent recipient intact for when the campaign resumes.
//
// Idempotent: the flush loop can see many blocked results across several
// batches, but only the first trips it. Without the guard, a batch of 50
// blocked recipients would issue 50 identical pause writes.
func (e *TaskExecutor) tripCircuitBreaker(ctx context.Context, cause error) {
	if !e.breakerTripped.CompareAndSwap(false, true) {
		return
	}

	taskId, err := e.getTaskIdFromContext(ctx)
	if err != nil {
		g.Log().Errorf(ctx, "SES account is blocked but the task id could not be resolved to pause it: %v", err)
		return
	}

	// The reason is stored against the task and shown to whoever is running the
	// campaign, so it stays provider-neutral and free of raw API errors. The
	// underlying cause is logged in full for whoever has to diagnose it.
	if cause != nil {
		g.Log().Errorf(ctx, "task %d: pausing, sending service reported: %v", taskId, cause)
	}

	reason := "Campaign paused automatically: the sending service is currently unable to accept mail. " +
		"No recipients have been lost -- the campaign will continue from where it stopped when you resume it. " +
		"Check the sending settings, then resume."

	if pauseErr := PauseTaskWithReason(ctx, taskId, reason); pauseErr != nil {
		g.Log().Errorf(ctx, "task %d: failed to pause after an account-level block: %v", taskId, pauseErr)
		return
	}

	// Stop dispatching immediately rather than waiting for the next loop check,
	// so no further recipients are pulled from the queue.
	e.isPaused.Store(true)
}

// checkSendQuotaBeforeStart pauses the task if its SES account does not have
// enough daily quota left to cover the recipients still waiting.
//
// Returns a non-nil error only when the task must not proceed. The task is
// paused rather than failed, so every unsent recipient stays at is_sent=0 and
// the campaign resumes untouched once the 24-hour window rolls or the limit is
// raised.
//
// Fails OPEN in two cases, both deliberate:
//   - the domain has no SES account, so there is no quota to check and the send
//     goes via SMTP anyway
//   - the quota lookup itself failed, e.g. AWS unreachable. Refusing to send
//     because we could not ask about the quota would turn a monitoring blip
//     into an outage, and the send path still handles a real rejection.
func (e *TaskExecutor) checkSendQuotaBeforeStart(ctx context.Context, taskId int, task *entity.EmailTask) error {
	// Count only recipients that are actually DUE right now -- the exact
	// population the send loop will attempt this run (getNextRecipientBatch
	// filters is_sent=0 AND sent_time <= now). Counting all is_sent=0 rows was
	// wrong in two ways:
	//
	//   - Warmup and retry-backoff park large numbers of recipients at
	//     is_sent=0 with sent_time days in the future. Counting them against a
	//     single day's remaining quota made the gate pause warmup campaigns
	//     that would have paced safely -- defeating the warmup subsystem it
	//     shares a codebase with.
	//   - A campaign whose recipients are all deferred (fully warmup-paced, or
	//     all in backoff) would report pending > 0 on every 5s scheduler
	//     relaunch, making a live AWS GetAccount call each time for a campaign
	//     that is not going to send anything this run.
	//
	// With the sent_time filter, a campaign with nothing due counts 0 and
	// returns below without touching AWS, and a warmup campaign is measured by
	// the batch actually about to go out. Tomorrow's quota is a fresh
	// allowance, so gating on the whole future campaign was never right anyway.
	now := time.Now().Unix()
	pending, err := g.DB().Model("recipient_info").
		Where("task_id", taskId).
		Where("is_sent", 0).
		Where("sent_time <= ?", now).
		Count()
	if err != nil {
		g.Log().Warningf(ctx, "task %d: could not count due recipients for the quota check: %v", taskId, err)
		return nil
	}
	if pending == 0 {
		return nil
	}

	quota, err := ses_api.CheckSendQuota(ctx, task.Addresser)
	if err != nil {
		g.Log().Warningf(ctx, "task %d: SES quota check failed, proceeding anyway: %v", taskId, err)
		return nil
	}
	if quota == nil {
		// No SES account for this domain -- nothing to gate on.
		return nil
	}

	if quota.Remaining() >= float64(pending) {
		g.Log().Infof(ctx, "task %d: SES quota check passed - %d pending, %.0f remaining in the 24h window",
			taskId, pending, quota.Remaining())
		return nil
	}

	reason := ses_api.DescribeQuotaShortfall(quota, pending)
	if pauseErr := PauseTaskWithReason(ctx, taskId, reason); pauseErr != nil {
		g.Log().Errorf(ctx, "task %d: quota is insufficient but pausing failed: %v", taskId, pauseErr)
		return pauseErr
	}
	e.isPaused.Store(true)

	return fmt.Errorf("task %d not started: %s", taskId, reason)
}

// safeGo runs fn in a goroutine that cannot take the process down with it.
//
// Go has no default recovery for goroutines: an unrecovered panic anywhere
// kills the whole process, not just that goroutine. The send path spawns
// several bare goroutines for statistics and bookkeeping, and a panic in any of
// them -- a nil map, an unexpected database shape -- would stop every campaign
// on the server, not merely lose one stats row.
//
// The stack is logged so a swallowed panic is still diagnosable. This is a
// safety net, not a licence to ignore errors: anything expected should still be
// returned and handled normally.
//
// IMPORTANT: recover() only catches panics. It does NOT catch fatal runtime
// errors such as "concurrent map iteration and map write" -- those terminate
// the process regardless. Only correct locking prevents those.
func safeGo(ctx context.Context, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				g.Log().Errorf(ctx, "recovered panic in %s: %v\n%s", name, r, debug.Stack())
			}
		}()
		fn()
	}()
}

// maxSendAttempts caps how many times a single recipient is tried before being
// written off.
//
// Deliberately small. The point is to survive a blip -- a throttle, a dropped
// connection, a brief service problem -- not to keep hammering a provider that
// is genuinely refusing. Permanent failures are never retried at all, so this
// budget only ever applies to errors that could plausibly clear.
const maxSendAttempts = 3

// retryBackoffSeconds returns how long to wait before retrying a recipient
// that has already failed attemptCount times.
//
// Exponential with a floor and a ceiling: 60s, then 300s, then 900s. The floor
// matters because the scheduler re-picks a task every 5 seconds, so without it
// a retry would fire almost immediately and simply fail again against whatever
// is still broken.
func retryBackoffSeconds(attemptCount int) int64 {
	switch {
	case attemptCount <= 0:
		return 60
	case attemptCount == 1:
		return 300
	default:
		return 900
	}
}

// failureGroup identifies a set of failures that can share one UPDATE: same
// attempt count, so the same backoff and the same next attempt number, and the
// same message, so the same last_error.
type failureGroup struct {
	attempt int
	errText string
}

// groupFailures buckets results so the flush can issue one statement per
// distinct outcome instead of one per recipient. During an outage every failure
// in a batch usually carries the same error and the same attempt count, so this
// normally collapses to a single group.
func groupFailures(results []*SendResult) map[failureGroup][]interface{} {
	groups := make(map[failureGroup][]interface{})
	for _, r := range results {
		key := failureGroup{attempt: r.AttemptCount, errText: truncateError(r.Error)}
		groups[key] = append(groups[key], r.RecipientID)
	}
	return groups
}

// truncateError renders an error for storage in recipient_info.last_error,
// bounded so a verbose AWS error cannot bloat the row.
func truncateError(err error) string {
	if err == nil {
		return ""
	}
	return truncateUTF8(err.Error(), 500)
}

// truncateUTF8 cuts s to at most maxBytes without splitting a rune.
//
// Plain byte slicing can land mid-rune and yield invalid UTF-8. Postgres
// rejects that outright in a text column ("invalid byte sequence for encoding
// UTF8"), so the UPDATE carrying it would fail -- and because that update is
// what moves a recipient out of the claimed state, the row would be stranded at
// is_sent=2 and the task could never complete. An error message quoting an
// internationalised address is enough to trigger it.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := s[:maxBytes]
	// Walk back over any trailing continuation bytes to land on a boundary.
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// recordSESFailureStats files a failed SES send in the mailstat tables so it
// shows up in bounce reporting and stays forensically recoverable.
//
// Call this ONLY when the message was genuinely not delivered -- not when the
// SMTP fallback subsequently succeeded, or the row would contradict reality.
//
// Parameters are passed explicitly rather than captured, because this runs in
// its own goroutine and the caller's locals are reused across recipients.
func (e *TaskExecutor) recordSESFailureStats(
	ctx context.Context,
	task *entity.EmailTask,
	recipient *entity.RecipientInfo,
	accountName string,
	messageID string,
	contentSize int,
	sendErr error,
) {
	sender := task.Addresser
	recipientAddr := recipient.Recipient

	errorDesc := "SES API error"
	if sendErr != nil {
		// Rune-safe: a mid-rune cut yields invalid UTF-8, which Postgres
		// rejects, losing the stats row entirely.
		errorDesc = truncateUTF8(sendErr.Error(), 200)
	}

	safeGo(ctx, "SES failure stats writer", func() {
		nowMillis := time.Now().UnixMilli()
		cleanMessageID := strings.Trim(messageID, "<>")
		sesPostfixID := sesStatsKey("ses-fail-", "", cleanMessageID)

		if _, err := g.DB().Model("mailstat_message_ids").InsertIgnore(g.Map{
			"postfix_message_id": sesPostfixID,
			"message_id":         cleanMessageID,
			"log_time_millis":    nowMillis,
		}); err != nil {
			g.Log().Warning(ctx, "Failed to insert SES failure message ID mapping:", err)
		}

		if _, err := g.DB().Model("mailstat_senders").InsertIgnore(g.Map{
			"postfix_message_id": sesPostfixID,
			"sender":             sender,
			"size":               contentSize,
			"log_time_millis":    nowMillis,
		}); err != nil {
			g.Log().Warning(ctx, "Failed to insert SES failure sender stats:", err)
		}

		if _, err := g.DB().Model("mailstat_send_mails").InsertIgnore(g.Map{
			"postfix_message_id": sesPostfixID,
			"recipient":          recipientAddr,
			"mail_provider":      public.GetMailProviderGroup(recipientAddr),
			"status":             "bounced",
			"delay":              0,
			"delays":             "0/0/0/0",
			"dsn":                "5.0.0",
			"relay":              "ses-api[" + accountName + "]",
			"description":        errorDesc,
			"log_time_millis":    nowMillis,
		}); err != nil {
			g.Log().Warning(ctx, "Failed to insert SES failure send stats:", err)
		}
	})
}

// sesStatsKey builds the synthetic postfix_message_id under which a SES send
// is filed in the mailstat tables.
//
// That column is a TEXT PRIMARY KEY and the inserts use InsertIgnore, so a
// duplicate key is silently discarded rather than erroring. The failure path
// previously keyed on millisecond timestamps, so two sends failing in the same
// millisecond -- routine with a concurrent worker pool -- lost one row and
// undercounted bounces.
//
// localMessageID is the per-message ID from generateMessageID, which carries
// 32 random characters and is therefore unique per send. It is used as the key
// on the failure path, and as a fallback on the success path when SES returns
// an empty MessageId (which would otherwise collapse every send in the
// campaign onto the single key "ses-").
func sesStatsKey(prefix, sesMessageID, localMessageID string) string {
	id := strings.TrimSpace(sesMessageID)
	if id == "" {
		id = localMessageID
	}
	return prefix + id
}

// generateMessageID generates a unique Message-ID for email
func generateMessageID(senderEmail string) string {
	randomID := grand.S(32)
	timestampMillis := time.Now().UnixMilli()

	domain := "tezmail"
	parts := strings.SplitN(senderEmail, "@", 2)
	if len(parts) > 1 {
		domain = parts[1]
	}

	return fmt.Sprintf("<%d.%s@%s>", timestampMillis, randomID, domain)
}

// sendEmailMock simulates sending an email and records it in the database.
func (e *TaskExecutor) sendEmailMock(ctx context.Context, task *entity.EmailTask, recipient *entity.RecipientInfo, content string) *SendResult {
	// Check if the context is canceled
	select {
	case <-ctx.Done():
		return &SendResult{
			RecipientID: recipient.Id,
			Success:     false,
			Error:       ctx.Err(),
		}
	default:
		// Continue execution
	}

	// Get the rendered content, subject and unsubscribe URL
	renderedContent, renderedSubject, _ := e.personalizeEmail(ctx, content, task, recipient)

	sender, err := mail_service.NewEmailSenderWithLocal(task.Addresser)
	if err != nil {
		g.Log().Error(ctx, "Failed to create email sender: %v", err)
		return &SendResult{
			RecipientID: recipient.Id,
			Success:     false,
			Error:       fmt.Errorf("failed to create email sender: %w", err),
		}
	}
	defer sender.Close()
	// Set message ID
	messageID := sender.GenerateMessageID()

	// Track email
	//baseURL := domains.GetBaseURLBySender(task.Addresser)
	baseURL := domains.GetBaseURL()
	mail_tracker := maillog_stat.NewMailTracker(renderedContent, task.Id, messageID, recipient.Recipient, baseURL)
	if task.TrackClick == 1 {
		mail_tracker.TrackLinks()
	}
	if task.TrackOpen == 1 {
		mail_tracker.AppendTrackingPixel()
	}
	renderedContent = mail_tracker.GetHTML()

	// Create email message with rendered subject
	message := mail_service.NewMessage(renderedSubject, renderedContent)
	message.SetMessageID(messageID)

	// Set sender display name
	if task.FullName != "" {
		message.SetRealName(task.FullName)
	}

	// We will create a log entry and save it, instead of sending.
	// This simulates a successful send.
	postfixMessageID := strings.ToUpper("TEST_" + grand.S(11))
	nowMillis := time.Now().UnixMilli()

	// 1. Create MailSender record
	senderRecord := &maillog_stat.MailSender{
		MailRecord: maillog_stat.MailRecord{
			PostfixMessageID: postfixMessageID,
			LogTimeMillis:    nowMillis,
		},
		Sender: task.Addresser,
		Size:   int64(len(renderedContent)),
	}
	_, err = g.DB().Model("mailstat_senders").InsertIgnore(senderRecord)
	if err != nil {
		g.Log().Debugf(ctx, "sendEmailMock: failed to insert mailstat_senders: %v", err)
	}

	// 2. Create MailMessageID record
	messageIDRecord := &maillog_stat.MailMessageID{
		MailRecord: maillog_stat.MailRecord{
			PostfixMessageID: postfixMessageID,
			LogTimeMillis:    nowMillis,
		},
		MessageID: strings.Trim(messageID, "<>"),
	}
	_, err = g.DB().Model("mailstat_message_ids").InsertIgnore(messageIDRecord)
	if err != nil {
		g.Log().Debugf(ctx, "sendEmailMock: failed to insert mailstat_message_ids: %v", err)
	}

	// 3. Create MailSendRecord record
	sendRecord := &maillog_stat.MailSendRecord{
		MailRecord: maillog_stat.MailRecord{
			PostfixMessageID: postfixMessageID,
			LogTimeMillis:    nowMillis,
		},
		Recipient:    recipient.Recipient,
		MailProvider: public.GetMailProviderGroup(recipient.Recipient),
		Status:       "sent",
		Delay:        0.1,              // Mock value
		Delays:       "0/0/0.1/0",      // Mock value
		Dsn:          "2.0.0",          // Mock value for successful send
		Relay:        "mock.relay.com", // Mock value
		Description:  "250 2.0.0 OK",   // Mock value
	}

	_, err = g.DB().Model("mailstat_send_mails").Data(sendRecord).Insert()
	if err != nil {
		g.Log().Errorf(ctx, "sendEmailMock: failed to insert mailstat_send_mails: %v", err)
		return &SendResult{
			RecipientID: recipient.Id,
			Success:     false,
			Error:       fmt.Errorf("sendEmailMock: failed to save record: %w", err),
		}
	}

	// 4. Simulate email open and click
	// Simulate a 50% open rate
	if grand.Intn(100) < 50 {
		openTimeMillis := nowMillis + int64(grand.Intn(3600*1000)) // Simulate opening within 1 hour of sending
		_, err = g.DB().Model("mailstat_opened").Insert(g.Map{
			"campaign_id":        task.Id,
			"log_time_millis":    openTimeMillis,
			"recipient":          recipient.Recipient,
			"message_id":         strings.Trim(messageID, "<>"),
			"postfix_message_id": postfixMessageID,
		})
		if err != nil {
			g.Log().Debugf(ctx, "sendEmailMock: failed to insert mailstat_opened: %v", err)
		}

		// If opened, simulate a 20% click rate
		if grand.Intn(100) < 20 {
			clickTimeMillis := openTimeMillis + int64(grand.Intn(600*1000)) // Simulate clicking within 10 minutes of opening
			_, err = g.DB().Model("mailstat_clicked").Insert(g.Map{
				"campaign_id":        task.Id,
				"log_time_millis":    clickTimeMillis,
				"recipient":          recipient.Recipient,
				"message_id":         strings.Trim(messageID, "<>"),
				"postfix_message_id": postfixMessageID,
			})
			if err != nil {
				g.Log().Debugf(ctx, "sendEmailMock: failed to insert mailstat_clicked: %v", err)
			}
		}
	}

	return &SendResult{
		RecipientID: recipient.Id,
		MessageID:   messageID,
		Success:     true,
		Error:       nil,
	}
}

// isTaskComplete check if task is complete
func (e *TaskExecutor) isTaskComplete(ctx context.Context, taskId int) (bool, error) {
	type CountResult struct {
		TotalCount int `json:"total_count"`
		SentCount  int `json:"sent_count"`
	}

	var result CountResult

	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {

		err := tx.Model("recipient_info").
			Fields("COUNT(1) as total_count, SUM(CASE WHEN is_sent = 1 THEN 1 ELSE 0 END) as sent_count").
			Where("task_id", taskId).
			Scan(&result)
		return err
	})

	if err != nil {
		return false, err
	}

	// if there are no recipients, task is not complete
	if result.TotalCount == 0 {
		return false, nil
	}

	// if sent count equals or exceeds total count, task is complete
	return result.SentCount >= result.TotalCount, nil
}

// GetMetrics get execution metrics
func (e *TaskExecutor) GetMetrics() map[string]interface{} {
	duration := time.Since(e.startTime).Seconds()
	sent := e.sentCount.Load()
	failed := e.failedCount.Load()
	total := sent + failed

	var successRate float64

	if total > 0 {
		successRate = float64(sent) / float64(total)
	}

	return map[string]interface{}{
		"sent_count":    sent,
		"failed_count":  failed,
		"total_count":   total,
		"success_rate":  successRate,
		"current_speed": e.rateController.GetCurrentRate(),
		"max_rate":      e.rateController.GetMaxRate(),
		"duration_sec":  duration,
	}
}

func (e *TaskExecutor) UpdateTaskThreads(taskId int, threads int) error {
	// parameter validation
	if threads <= 0 {
		return fmt.Errorf("threads must be greater than zero")
	}

	if threads > 100 {
		return fmt.Errorf("threads must be less than 100")
	}

	// get task info
	task, err := GetTaskInfo(context.Background(), taskId)
	if err != nil {
		return fmt.Errorf("get task info failed: %w", err)
	}

	if task == nil || task.Id == 0 {
		return fmt.Errorf("task %d not found", taskId)
	}

	// record current pool status
	var oldPoolSize int
	var runningWorkers int
	if e.pool != nil {
		oldPoolSize = e.pool.Cap()
		runningWorkers = e.pool.Running()
	}

	// new threads
	newThreads := threads
	// calculate new rate limit - 20 emails per thread per second
	targetSendPerThreadPerSecond := 20
	newRate := newThreads * targetSendPerThreadPerSecond * 60

	// create new rate controller
	e.rateController = NewSimpleRateController(newRate)

	// if task is running, adjust pool size
	if e.pool != nil && e.IsRunning() {
		// if new pool size is not equal to current pool size, create new pool
		if newThreads != oldPoolSize {
			// if request to decrease capacity, but current running number is close to new capacity, output warning
			if newThreads < oldPoolSize && runningWorkers > int(float64(newThreads)*0.8) {
				warningMsg := fmt.Sprintf("task %d: request pool size (%d) is less than current running workers (%d), may cause task queue",
					taskId, newThreads, runningWorkers)
				g.Log().Warning(context.Background(), warningMsg)
			}

			// create new pool
			newPool, err := ants.NewPool(newThreads,
				ants.WithPreAlloc(true),
				ants.WithPanicHandler(func(p interface{}) {
					g.Log().Error(context.Background(), "Worker panic: %v", p)
				}),
				ants.WithMaxBlockingTasks(newThreads*200),
				ants.WithNonblocking(false))

			if err != nil {
				g.Log().Error(context.Background(), "task %d: create new pool failed: %v", taskId, err)
				// even if creating new pool failed, we will still update rate controller
			} else {
				// get old pool reference
				oldPool := e.pool

				// replace with new pool
				e.pool = newPool

				// safely close old pool
				go func(pool *ants.Pool, oldSize int, oldRunning int) {
					// calculate wait time - adjust dynamically based on current active workers
					waitTime := 1 * time.Second
					if oldRunning > 0 {
						// add 1 second for every 10 active workers, minimum 1 second, maximum 10 seconds
						waitSecs := 1 + oldRunning/10
						if waitSecs > 10 {
							waitSecs = 10
						}
						waitTime = time.Duration(waitSecs) * time.Second
					}

					waitInfoMsg := fmt.Sprintf("task %d: wait %d seconds to release old pool (running: %d/%d)",
						taskId, int(waitTime.Seconds()), oldRunning, oldSize)
					g.Log().Info(context.Background(), waitInfoMsg)

					time.Sleep(waitTime)
					pool.Release()
					releaseMsg := fmt.Sprintf("task %d: old pool released", taskId)
					g.Log().Info(context.Background(), releaseMsg)
				}(oldPool, oldPoolSize, runningWorkers)
			}
		} else {
			keepMsg := fmt.Sprintf("task %d: pool size keep unchanged (%d), only adjust rate controller", taskId, oldPoolSize)
			g.Log().Info(context.Background(), keepMsg)
		}
	}

	// update threads in database
	_, err = g.DB().Model("email_tasks").
		Where("id", taskId).
		Data(g.Map{"threads": newThreads}).
		Update()

	if err != nil {

		return fmt.Errorf("task %d: update database threads failed: %w", taskId, err)
	}

	return nil
}
