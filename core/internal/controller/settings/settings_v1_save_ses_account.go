package settings

import (
	"billionmail-core/api/settings/v1"
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/ses_api"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (c *ControllerV1) SaveSESAccount(ctx context.Context, req *v1.SaveSESAccountReq) (res *v1.SaveSESAccountRes, err error) {
	res = &v1.SaveSESAccountRes{}

	// Validate required fields
	if req.Name == "" {
		res.SetError(gerror.New(public.LangCtx(ctx, "Account name is required")))
		return res, nil
	}
	if req.Region == "" {
		res.SetError(gerror.New(public.LangCtx(ctx, "AWS region is required")))
		return res, nil
	}
	if req.AccessKey == "" {
		res.SetError(gerror.New(public.LangCtx(ctx, "AWS access key is required")))
		return res, nil
	}
	// Secret key required only for new accounts
	if req.Id == 0 && req.SecretKey == "" {
		res.SetError(gerror.New(public.LangCtx(ctx, "AWS secret key is required for new accounts")))
		return res, nil
	}

	// Default check interval
	if req.CheckIntervalMinutes <= 0 {
		req.CheckIntervalMinutes = 30
	}

	account := &v1.SESAccount{
		Id:                   req.Id,
		Name:                 req.Name,
		Description:          req.Description,
		Region:               req.Region,
		AccessKey:            req.AccessKey,
		SecretKey:            req.SecretKey,
		Enabled:              req.Enabled,
		CheckIntervalMinutes: req.CheckIntervalMinutes,
		Domains:              req.Domains,
	}

	savedAccount, err := ses_api.SaveAccountToDB(ctx, account)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to save SES account: {}", err)))
		return res, nil
	}
	if savedAccount == nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to retrieve saved account")))
		return res, nil
	}

	// Log the operation
	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Settings,
		Log:  "Updated SES account configuration",
		Data: map[string]interface{}{
			"account_name": req.Name,
			"region":       req.Region,
			"enabled":      req.Enabled,
		},
	})

	res.Data.Account = *savedAccount
	res.SetSuccess(public.LangCtx(ctx, "SES account saved successfully"))
	return res, nil
}
