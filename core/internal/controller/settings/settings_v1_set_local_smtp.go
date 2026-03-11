package settings

import (
	"billionmail-core/internal/service/public"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"billionmail-core/api/settings/v1"
)

func (c *ControllerV1) SetLocalSMTP(ctx context.Context, req *v1.SetLocalSMTPReq) (res *v1.SetLocalSMTPRes, err error) {
	res = &v1.SetLocalSMTPRes{}

	g.Log().Infof(ctx, "[LOCAL-SMTP-GUARD] SetLocalSMTP called: enabled=%v", req.Enabled)

	err = public.OptionsMgrInstance.SetOption(ctx, "local_smtp_enabled", req.Enabled)
	if err != nil {
		g.Log().Error(ctx, "[LOCAL-SMTP-GUARD] SetLocalSMTP: failed to save setting:", err)
		res.SetError(gerror.Newf(public.LangCtx(ctx, "Failed to set local SMTP switch: %v", err)))
		return res, nil
	}

	g.Log().Infof(ctx, "[LOCAL-SMTP-GUARD] SetLocalSMTP: successfully set local_smtp_enabled=%v", req.Enabled)
	res.SetSuccess(public.LangCtx(ctx, "Local SMTP switch updated successfully"))
	return res, nil
}
