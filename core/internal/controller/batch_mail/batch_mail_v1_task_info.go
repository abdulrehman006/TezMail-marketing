package batch_mail

import (
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/public"
	"context"
	"encoding/json"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"billionmail-core/api/batch_mail/v1"
)

func (c *ControllerV1) TaskInfo(ctx context.Context, req *v1.TaskInfoReq) (res *v1.TaskInfoRes, err error) {
	res = &v1.TaskInfoRes{}

	var taskInfo *entity.EmailTask
	taskInfo, err = batch_mail.GetTaskInfo(ctx, req.Id)
	if err != nil {
		res.Code = 500
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to get task info: {}", err.Error())))
		return
	}

	if taskInfo == nil || taskInfo.Id == 0 {
		res.Code = 404
		res.SetError(gerror.New(public.LangCtx(ctx, "Task not found: {}", req.Id)))
		return
	}

	// Convert tag_ids string to array before returning
	if taskInfo.TagIdsRaw != "" {
		var tagIds []int
		if err := json.Unmarshal([]byte(taskInfo.TagIdsRaw), &tagIds); err == nil {
			taskInfo.TagIds = tagIds
		}
	}

	// Check if warmup is enabled for this task by looking up bm_campaign_warmup table
	count, _ := g.DB().Ctx(ctx).Model("bm_campaign_warmup").Where("task_id", req.Id).Count()
	if count > 0 {
		taskInfo.Warmup = 1
	}

	res.Data = taskInfo
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return
}
