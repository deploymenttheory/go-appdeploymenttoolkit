package procmgmt

import (
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// TokenSelection picks which token variant of the target user a LaunchAsUser
// call runs under, porting Start-ADTProcessAsUser's -UseLinkedAdminToken /
// -UseHighestAvailableToken / -UseUnelevatedToken switches (TokenManager.cs).
type TokenSelection int

// TokenSelection values.
const (
	// TokenDefault uses the session's primary token as-is.
	TokenDefault TokenSelection = iota
	// TokenLinkedAdmin requires the linked (elevated) admin token; the
	// launch fails when the user has no split token.
	TokenLinkedAdmin
	// TokenHighestAvailable uses the linked admin token when the user has
	// one, else the base token.
	TokenHighestAvailable
	// TokenUnelevated uses the limited token even when the base token is
	// elevated.
	TokenUnelevated
)

// selectUserToken derives the requested token variant from the user's primary
// token. The returned token is always a new primary token the caller must
// close, so the input token is never aliased.
func selectUserToken(primary windows.Token, sel TokenSelection) (windows.Token, error) {
	switch sel {
	case TokenLinkedAdmin:
		linked, err := linkedPrimaryToken(primary)
		if err != nil {
			return 0, fmt.Errorf("procmgmt: linked admin token unavailable: %w", err)
		}
		return linked, nil
	case TokenHighestAvailable:
		if primary.IsElevated() {
			return duplicatePrimaryToken(primary)
		}
		if linked, err := linkedPrimaryToken(primary); err == nil {
			return linked, nil
		}
		return duplicatePrimaryToken(primary)
	case TokenUnelevated:
		if !primary.IsElevated() {
			return duplicatePrimaryToken(primary)
		}
		linked, err := linkedPrimaryToken(primary)
		if err != nil {
			return 0, fmt.Errorf("procmgmt: unelevated (limited) token unavailable: %w", err)
		}
		return linked, nil
	case TokenDefault:
		return duplicatePrimaryToken(primary)
	default:
		return 0, fmt.Errorf("procmgmt: TokenSelection %d out of range: %w", sel, winerr.ErrInvalidOption)
	}
}

// linkedPrimaryToken returns the token's UAC-linked counterpart as a primary
// token (elevated for a limited token, limited for an elevated one).
func linkedPrimaryToken(t windows.Token) (windows.Token, error) {
	linked, err := t.GetLinkedToken()
	if err != nil {
		return 0, fmt.Errorf("procmgmt: GetLinkedToken: %w", err)
	}
	defer func() { _ = linked.Close() }()
	return duplicatePrimaryToken(linked)
}

// duplicatePrimaryToken duplicates any token into a fresh primary token.
func duplicatePrimaryToken(t windows.Token) (windows.Token, error) {
	var dup windows.Token
	if err := windows.DuplicateTokenEx(
		t,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&dup,
	); err != nil {
		return 0, fmt.Errorf("procmgmt: DuplicateTokenEx: %w", err)
	}
	return dup, nil
}
