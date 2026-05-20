package api

import "github.com/pquerna/otp/totp"

// validateTOTP wraps the otp library so callers don't need to import it.
func validateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}
