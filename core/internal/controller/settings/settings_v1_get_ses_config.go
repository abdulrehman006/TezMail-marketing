package settings

import (
	"billionmail-core/api/settings/v1"
	"billionmail-core/internal/service/ses_api"
	"context"
)

func (c *ControllerV1) GetSESConfig(ctx context.Context, req *v1.GetSESConfigReq) (res *v1.GetSESConfigRes, err error) {
	res = &v1.GetSESConfigRes{}

	accounts, err := ses_api.GetAllAccountsFromDB(ctx)
	if err != nil {
		res.SetError(err)
		return res, nil
	}

	res.Data.Accounts = accounts
	res.SetSuccess("")
	return res, nil
}
