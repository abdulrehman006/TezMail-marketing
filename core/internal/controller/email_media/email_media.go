// =================================================================================
// Shared email-media library controller.
// =================================================================================

package email_media

import (
	"billionmail-core/api/email_media"
)

// ControllerV1 email-media controller
type ControllerV1 struct{}

// NewV1 creates the email-media v1 controller
func NewV1() email_media.IEmailMediaV1 {
	return &ControllerV1{}
}
