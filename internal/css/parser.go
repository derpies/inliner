package css

import (
	"regexp"
	"strings"
)

// Parser handles CSS parsing and specificity calculation
type Parser struct {
	// Regular expressions for CSS parsing
	ruleRegex        *regexp.Regexp
	declarationRegex *regexp.Regexp
	importantRegex   *regexp.Regexp

	// Specificity calculation regexes
	idRegex            *regexp.Regexp
	classRegex         *regexp.Regexp
	attrRegex          *regexp.Regexp
	pseudoClassRegex   *regexp.Regexp
	elementRegex       *regexp.Regexp
	pseudoElementRegex *regexp.Regexp
}

// NewParser creates a new CSS parser with compiled regexes
func NewParser() *Parser {
	return &Parser{
		// CSS rule parsing: selector { declarations }
		ruleRegex: regexp.MustCompile(`([^{]+)\{([^}]*)\}`),

		// Declaration parsing: property: value; or property: value !important;
		declarationRegex: regexp.MustCompile(`([^:]+):\s*([^;]+);?`),
		importantRegex:   regexp.MustCompile(`!\s*important\s*$`),

		// Specificity calculation regexes (RE2 compatible)
		idRegex:            regexp.MustCompile(`#[a-zA-Z0-9_-]+`),
		classRegex:         regexp.MustCompile(`\.[a-zA-Z0-9_-]+`),
		attrRegex:          regexp.MustCompile(`\[[^\]]*\]`),
		pseudoClassRegex:   regexp.MustCompile(`:[a-zA-Z0-9_-]+`),
		elementRegex:       regexp.MustCompile(`\b[a-zA-Z]+\b`),
		pseudoElementRegex: regexp.MustCompile(`::[a-zA-Z0-9_-]+`),
	}
}

// Parse parses CSS text into a Stylesheet
func (p *Parser) Parse(cssText string) (*Stylesheet, error) {
	stylesheet := &Stylesheet{
		Rules: make([]Rule, 0),
	}

	// Remove comments
	cssText = p.removeComments(cssText)

	// Handle @media, @keyframes, etc. - for now, preserve them as-is
	// TODO: Add proper at-rule parsing in next iteration
	cssText = p.extractAtRules(cssText)

	// Find all CSS rules
	matches := p.ruleRegex.FindAllStringSubmatch(cssText, -1)

	for i, match := range matches {
		if len(match) != 3 {
			continue
		}

		selector := strings.TrimSpace(match[1])
		declarations := strings.TrimSpace(match[2])

		// Skip empty rules
		if selector == "" || declarations == "" {
			continue
		}

		// Parse declarations
		parsedDeclarations, err := p.parseDeclarations(declarations)
		if err != nil {
			// Log error but continue parsing other rules
			continue
		}

		// Calculate specificity
		specificity := p.calculateSpecificity(selector)

		rule := Rule{
			Selector:     selector,
			Specificity:  specificity,
			Declarations: parsedDeclarations,
			SourceOrder:  i,
		}

		stylesheet.Rules = append(stylesheet.Rules, rule)
	}

	return stylesheet, nil
}

// parseDeclarations parses CSS declarations from a declaration block
func (p *Parser) parseDeclarations(declarationsText string) (map[string]Declaration, error) {
	declarations := make(map[string]Declaration)

	// Split by semicolon, but handle semicolons in quoted strings
	parts := p.smartSplit(declarationsText, ';')

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Find the first colon that's not in a quoted string
		colonIndex := p.findUnquotedChar(part, ':')
		if colonIndex == -1 {
			continue
		}

		property := strings.TrimSpace(part[:colonIndex])
		value := strings.TrimSpace(part[colonIndex+1:])

		if property == "" || value == "" {
			continue
		}

		// Check for !important
		important := p.importantRegex.MatchString(value)
		if important {
			value = p.importantRegex.ReplaceAllString(value, "")
			value = strings.TrimSpace(value)
		}

		// Normalize property name to lowercase
		property = strings.ToLower(property)

		declarations[property] = Declaration{
			Property:  property,
			Value:     value,
			Important: important,
		}
	}

	return declarations, nil
}

// calculateSpecificity calculates CSS specificity according to CSS specification
func (p *Parser) calculateSpecificity(selector string) Specificity {
	spec := Specificity{}

	// Simple approach - count occurrences of each selector type
	// This is less precise but RE2 compatible

	// Count IDs
	spec.IDs = len(p.idRegex.FindAllString(selector, -1))

	// Count classes
	spec.Classes += len(p.classRegex.FindAllString(selector, -1))

	// Count attributes
	spec.Classes += len(p.attrRegex.FindAllString(selector, -1))

	// Count pseudo-classes (but filter out pseudo-elements)
	pseudoMatches := p.pseudoClassRegex.FindAllString(selector, -1)
	for _, match := range pseudoMatches {
		// Only count single colons, not double colons
		if !strings.HasPrefix(match, "::") {
			spec.Classes++
		}
	}

	// Count pseudo-elements
	spec.Elements += len(p.pseudoElementRegex.FindAllString(selector, -1))

	// Count element types
	elementMatches := p.elementRegex.FindAllString(selector, -1)
	for _, match := range elementMatches {
		element := strings.ToLower(match)
		// Skip pseudo-class keywords and CSS keywords
		if !p.isPseudoClassKeyword(element) && !p.isCSSKeyword(element) {
			spec.Elements++
		}
	}

	return spec
}

// isPseudoClassKeyword checks if a string is a pseudo-class keyword
func (p *Parser) isPseudoClassKeyword(s string) bool {
	pseudoClasses := map[string]bool{
		"hover": true, "focus": true, "active": true, "visited": true,
		"link": true, "first-child": true, "last-child": true,
		"nth-child": true, "nth-of-type": true, "not": true,
	}
	return pseudoClasses[s]
}

// isCSSKeyword checks if a string is a CSS keyword that shouldn't count as an element
func (p *Parser) isCSSKeyword(s string) bool {
	keywords := map[string]bool{
		"and": true, "or": true, "not": true, "only": true,
		"all": true, "screen": true, "print": true,
	}
	return keywords[s]
}

// removeComments removes CSS comments /* ... */
func (p *Parser) removeComments(css string) string {
	commentRegex := regexp.MustCompile(`/\*[^*]*\*+([^/*][^*]*\*+)*/`)
	return commentRegex.ReplaceAllString(css, "")
}

// extractAtRules handles @media, @keyframes, etc. - for now just remove them
// TODO: Implement proper at-rule preservation for media queries
func (p *Parser) extractAtRules(css string) string {
	// Simple implementation for now - remove @import, @charset, etc.
	// Preserve @media for later processing
	atRuleRegex := regexp.MustCompile(`@(import|charset|namespace)[^;]+;`)
	return atRuleRegex.ReplaceAllString(css, "")
}

// smartSplit splits a string by delimiter, respecting quoted strings
func (p *Parser) smartSplit(s string, delimiter rune) []string {
	var parts []string
	var current strings.Builder
	var inQuotes bool
	var quoteChar rune

	for _, char := range s {
		switch {
		case !inQuotes && (char == '"' || char == '\''):
			inQuotes = true
			quoteChar = char
			current.WriteRune(char)
		case inQuotes && char == quoteChar:
			inQuotes = false
			current.WriteRune(char)
		case !inQuotes && char == delimiter:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// findUnquotedChar finds the first occurrence of char that's not in quotes
func (p *Parser) findUnquotedChar(s string, char rune) int {
	var inQuotes bool
	var quoteChar rune

	for i, c := range s {
		switch {
		case !inQuotes && (c == '"' || c == '\''):
			inQuotes = true
			quoteChar = c
		case inQuotes && c == quoteChar:
			inQuotes = false
		case !inQuotes && c == char:
			return i
		}
	}

	return -1
}

// ParseInlineStyle parses inline style attribute into declarations
func (p *Parser) ParseInlineStyle(styleAttr string) (map[string]Declaration, error) {
	// Inline styles have maximum specificity (1000, 0, 0, 0)
	return p.parseDeclarations(styleAttr)
}

// NormalizePropertyName normalizes CSS property names
func NormalizePropertyName(property string) string {
	// Convert to lowercase and trim
	property = strings.ToLower(strings.TrimSpace(property))

	// Handle vendor prefixes consistently
	if strings.HasPrefix(property, "-webkit-") ||
		strings.HasPrefix(property, "-moz-") ||
		strings.HasPrefix(property, "-ms-") ||
		strings.HasPrefix(property, "-o-") {
		return property
	}

	return property
}

// IsEmailSafeProperty checks if a CSS property is safe for email clients
func IsEmailSafeProperty(property string) bool {
	// List of properties that work reliably across email clients
	safeProperties := map[string]bool{
		// Text properties
		"color":           true,
		"font-family":     true,
		"font-size":       true,
		"font-weight":     true,
		"font-style":      true,
		"text-align":      true,
		"text-decoration": true,
		"line-height":     true,
		"letter-spacing":  true,

		// Box model
		"width":          true,
		"height":         true,
		"padding":        true,
		"padding-top":    true,
		"padding-right":  true,
		"padding-bottom": true,
		"padding-left":   true,
		"margin":         true,
		"margin-top":     true,
		"margin-right":   true,
		"margin-bottom":  true,
		"margin-left":    true,

		// Background
		"background":       true,
		"background-color": true,
		"background-image": true,

		// Border
		"border":        true,
		"border-top":    true,
		"border-right":  true,
		"border-bottom": true,
		"border-left":   true,
		"border-color":  true,
		"border-style":  true,
		"border-width":  true,

		// Table properties
		"border-collapse": true,
		"border-spacing":  true,
		"vertical-align":  true,
	}

	return safeProperties[strings.ToLower(property)]
}

// SpecificityFromInline creates a specificity for inline styles
func SpecificityFromInline(important bool) Specificity {
	return Specificity{
		Inline:    1000,
		IDs:       0,
		Classes:   0,
		Elements:  0,
		Important: important,
	}
}
