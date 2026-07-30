package batch_mail

import (
	"billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/public"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

// RecipientPreview returns the number of UNIQUE recipients a campaign would reach
// across the selected list(s), de-duplicated by email — so the UI never
// double-counts a contact who belongs to more than one selected list. It uses
// the exact same resolver as task creation, so the preview matches the send.
func (c *ControllerV1) RecipientPreview(ctx context.Context, req *v1.RecipientPreviewReq) (res *v1.RecipientPreviewRes, err error) {
	res = &v1.RecipientPreviewRes{}

	// Effective list ids: GroupIds (multi) plus GroupId for backward
	// compatibility. De-duplication of ids is handled inside the service.
	ids := append(append([]int{}, req.GroupIds...), req.GroupId)

	total, e := batch_mail.CountCampaignContacts(ctx, ids, req.TagIds, req.TagLogic)
	if e != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to count recipients: {}", e.Error())))
		return
	}

	res.Data.Total = total
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return
}
