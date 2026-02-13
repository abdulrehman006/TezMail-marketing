package settings

import (
	"billionmail-core/api/settings/v1"
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/ses_api"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (c *ControllerV1) DeleteSESAccount(ctx context.Context, req *v1.DeleteSESAccountReq) (res *v1.DeleteSESAccountRes, err error) {
	res = &v1.DeleteSESAccountRes{}

	if req.Id <= 0 {
		res.SetError(gerror.New(public.LangCtx(ctx, "Invalid account ID")))
		return res, nil
	}

	err = ses_api.DeleteAccountFromDB(ctx, req.Id)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to delete SES account: {}", err)))
		return res, nil
	}

	// Log the operation
	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Settings,
		Log:  "Deleted SES account",
		Data: map[string]interface{}{
			"account_id": req.Id,
		},
	})

	res.SetSuccess(public.LangCtx(ctx, "SES account deleted successfully"))
	return res, nil
}
