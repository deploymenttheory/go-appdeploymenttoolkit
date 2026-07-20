package winadt

// NewADTLogFileName is the Go port of New-ADTLogFileName: it returns the
// active session's default log file name with the given discriminator.
func NewADTLogFileName(discriminator string) (string, error) {
	s, err := GetADTSession()
	if err != nil {
		return "", err
	}
	return s.NewLogFileName(discriminator), nil
}
