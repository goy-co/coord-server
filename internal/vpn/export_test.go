package vpn

// SetTailscaleClientBaseURL allows overriding the base URL of TailscaleClient for testing.
func SetTailscaleClientBaseURL(c *TailscaleClient, url string) {
	c.baseURL = url
}
