package config

import "strings"

// Config holds configuration options for the inlining process
type Config struct {
	// PreserveMediaQueries keeps @media rules in <style> tags
	PreserveMediaQueries bool

	// PreservePseudoSelectors keeps :hover, :focus, etc. in <style> tags
	PreservePseudoSelectors bool

	// RemoveStyleTags removes <style> tags after inlining (use with caution)
	RemoveStyleTags bool

	// StripUnusedCSS removes CSS rules that don't match any elements
	StripUnusedCSS bool

	// EmailClientOptimizations applies email client specific optimizations
	EmailClientOptimizations bool

	// PreserveWhitespace maintains original HTML formatting
	PreserveWhitespace bool

	// TargetEmailClient optimizes for specific email client
	TargetEmailClient string
}

// Default returns a configuration optimized for email clients
func Default() Config {
	return Config{
		PreserveMediaQueries:     true,      // Needed for responsive emails
		PreservePseudoSelectors:  true,      // :hover states for buttons
		RemoveStyleTags:          false,     // Keep for email client compatibility
		StripUnusedCSS:           true,      // Reduce email size
		EmailClientOptimizations: true,      // Apply email-specific fixes
		PreserveWhitespace:       true,      // Maintain email formatting
		TargetEmailClient:        "generic", // Conservative defaults
	}
}

// EmailClientCompatibility holds information about email client CSS support
type EmailClientCompatibility struct {
	SupportsMediaQueries    bool
	SupportsPseudoSelectors map[string]bool // :hover, :focus, etc.
	RequiresInlineStyles    bool
	MaxStylesheetSize       int // in bytes, 0 = no limit
}

// GetCompatibilityProfile returns compatibility info for major email clients
func GetCompatibilityProfile(client string) EmailClientCompatibility {
	switch strings.ToLower(client) {
	case "outlook", "outlook_desktop":
		return EmailClientCompatibility{
			SupportsMediaQueries:    false, // Desktop Outlook uses Word engine
			SupportsPseudoSelectors: map[string]bool{":hover": false, ":focus": false},
			RequiresInlineStyles:    true,
			MaxStylesheetSize:       65536, // 64KB limit
		}
	case "gmail", "gmail_web":
		return EmailClientCompatibility{
			SupportsMediaQueries:    true,
			SupportsPseudoSelectors: map[string]bool{":hover": true, ":focus": true},
			RequiresInlineStyles:    false, // But still recommended
			MaxStylesheetSize:       0,     // No hard limit
		}
	case "apple_mail", "mail_app":
		return EmailClientCompatibility{
			SupportsMediaQueries:    true,
			SupportsPseudoSelectors: map[string]bool{":hover": true, ":focus": true},
			RequiresInlineStyles:    false,
			MaxStylesheetSize:       0,
		}
	case "outlook_online", "outlook_web":
		return EmailClientCompatibility{
			SupportsMediaQueries:    true, // Web version is better
			SupportsPseudoSelectors: map[string]bool{":hover": true, ":focus": false},
			RequiresInlineStyles:    true, // Still recommended
			MaxStylesheetSize:       65536,
		}
	default:
		// Conservative defaults for unknown clients
		return EmailClientCompatibility{
			SupportsMediaQueries:    false,
			SupportsPseudoSelectors: map[string]bool{},
			RequiresInlineStyles:    true,
			MaxStylesheetSize:       32768, // 32KB conservative limit
		}
	}
}
