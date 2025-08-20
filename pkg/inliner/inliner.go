package inliner

import (
	"fmt"
	"strings"

	"inliner/internal/config"
	"inliner/internal/css"
	"inliner/internal/html"
	"inliner/internal/resolver"
)

// Inliner is the main CSS inlining engine for email HTML
type Inliner struct {
	config     config.Config
	parser     *css.Parser
	htmlParser html.Parser
}

// New creates a new CSS inliner with the given configuration
func New(cfg config.Config) *Inliner {
	return &Inliner{
		config:     cfg,
		parser:     css.NewParser(),
		htmlParser: html.NewParser(),
	}
}

// NewWithDefaults creates a new CSS inliner with email-optimized defaults
func NewWithDefaults() *Inliner {
	return New(config.Default())
}

// InlineResult contains the result of CSS inlining operation
type InlineResult struct {
	HTML            string                       // Final HTML with inlined styles
	InlinedStyles   int                          // Number of styles successfully inlined
	PreservedRules  int                          // Number of CSS rules preserved in <style> tags
	Warnings        []resolver.ValidationWarning // Any validation warnings
	ProcessingStats ProcessingStats              // Performance and processing statistics
}

// ProcessingStats contains performance metrics from the inlining process
type ProcessingStats struct {
	CSSRulesParsed        int   // Total CSS rules parsed
	HTMLElementsProcessed int   // HTML elements that had styles applied
	SelectorsMatched      int   // Total selector matches found
	ProcessingTimeMs      int64 // Processing time in milliseconds
}

// Inline processes HTML with embedded or external CSS and inlines styles
func (i *Inliner) Inline(htmlContent string) (*InlineResult, error) {
	// Parse the HTML document
	doc, err := i.htmlParser.Parse(htmlContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Extract CSS from <style> tags and external stylesheets
	cssContent, err := i.extractCSS(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to extract CSS: %w", err)
	}

	// Parse the CSS
	stylesheet, err := i.parser.Parse(cssContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSS: %w", err)
	}

	// Create style resolver
	styleResolver := resolver.New(stylesheet, i.config)

	// Process all elements in the document
	result, err := i.processDocument(doc, styleResolver)
	if err != nil {
		return nil, fmt.Errorf("failed to process document: %w", err)
	}

	// Handle style tag cleanup/preservation
	if err := i.handleStyleTags(doc, stylesheet); err != nil {
		return nil, fmt.Errorf("failed to handle style tags: %w", err)
	}

	// Generate final HTML
	finalHTML, err := doc.HTML()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize HTML: %w", err)
	}

	result.HTML = finalHTML
	return result, nil
}

// InlineString is a convenience method that inlines CSS in an HTML string
func (i *Inliner) InlineString(htmlContent string) (string, error) {
	result, err := i.Inline(htmlContent)
	if err != nil {
		return "", err
	}
	return result.HTML, nil
}

// extractCSS extracts all CSS content from the document
func (i *Inliner) extractCSS(doc html.Document) (string, error) {
	var cssContent strings.Builder

	// Extract from <style> tags
	styleTags, err := doc.GetStyleTags()
	if err != nil {
		return "", fmt.Errorf("failed to get style tags: %w", err)
	}

	for _, styleTag := range styleTags {
		content := styleTag.Text()
		if content != "" {
			cssContent.WriteString(content)
			cssContent.WriteString("\n")
		}
	}

	// TODO: Extract from <link rel="stylesheet"> tags
	// TODO: Extract from external CSS files

	return cssContent.String(), nil
}

// processDocument processes all elements in the document and applies inline styles
func (i *Inliner) processDocument(doc html.Document, styleResolver *resolver.Resolver) (*InlineResult, error) {
	result := &InlineResult{
		ProcessingStats: ProcessingStats{},
		Warnings:        []resolver.ValidationWarning{},
	}

	// Get all elements in the document
	allElements, err := doc.QuerySelectorAll("*")
	if err != nil {
		return nil, fmt.Errorf("failed to query all elements: %w", err)
	}

	result.ProcessingStats.HTMLElementsProcessed = len(allElements)

	// Process each element
	for _, element := range allElements {
		if err := i.processElement(element, styleResolver, result); err != nil {
			// Log error but continue processing other elements
			continue
		}
	}

	return result, nil
}

// processElement processes a single HTML element and applies computed styles
func (i *Inliner) processElement(element html.Node, styleResolver *resolver.Resolver, result *InlineResult) error {
	// Skip certain elements that shouldn't have styles
	tagName := strings.ToLower(element.TagName())
	if i.shouldSkipElement(tagName) {
		return nil
	}

	// Resolve styles for this element
	computedStyles, err := styleResolver.ResolveStyles(element)
	if err != nil {
		return fmt.Errorf("failed to resolve styles for %s: %w", tagName, err)
	}

	// Skip if no styles computed
	if len(computedStyles) == 0 {
		return nil
	}

	// Get existing inline styles
	existingStyles := element.GetInlineStyle()

	// Merge computed styles with existing inline styles
	finalStyles := styleResolver.MergeStyles(existingStyles, computedStyles)

	// Validate styles for email compatibility
	warnings := styleResolver.ValidateStyles(finalStyles)
	result.Warnings = append(result.Warnings, warnings...)

	// Apply the final styles to the element
	if err := element.SetInlineStyle(finalStyles); err != nil {
		return fmt.Errorf("failed to set inline styles: %w", err)
	}

	result.InlinedStyles += len(finalStyles)
	return nil
}

// shouldSkipElement determines if an element should be skipped during processing
func (i *Inliner) shouldSkipElement(tagName string) bool {
	skipTags := map[string]bool{
		"html":     true,
		"head":     true,
		"title":    true,
		"meta":     true,
		"link":     true,
		"script":   true,
		"style":    true,
		"noscript": true,
		"base":     true,
	}

	return skipTags[tagName]
}

// handleStyleTags manages <style> tags based on configuration
func (i *Inliner) handleStyleTags(doc html.Document, stylesheet *css.Stylesheet) error {
	styleTags, err := doc.GetStyleTags()
	if err != nil {
		return fmt.Errorf("failed to get style tags: %w", err)
	}

	if i.config.RemoveStyleTags {
		// Remove all style tags
		for _, styleTag := range styleTags {
			if err := i.removeElement(styleTag); err != nil {
				return fmt.Errorf("failed to remove style tag: %w", err)
			}
		}
		return nil
	}

	// Preserve certain CSS rules in style tags
	preservedCSS := i.buildPreservedCSS(stylesheet)

	if preservedCSS != "" {
		// Update the first style tag with preserved CSS, remove others
		if len(styleTags) > 0 {
			if err := i.updateStyleTagContent(styleTags[0], preservedCSS); err != nil {
				return fmt.Errorf("failed to update style tag: %w", err)
			}

			// Remove additional style tags
			for j := 1; j < len(styleTags); j++ {
				if err := i.removeElement(styleTags[j]); err != nil {
					return fmt.Errorf("failed to remove extra style tag: %w", err)
				}
			}
		}
	} else {
		// No CSS to preserve, remove all style tags
		for _, styleTag := range styleTags {
			if err := i.removeElement(styleTag); err != nil {
				return fmt.Errorf("failed to remove style tag: %w", err)
			}
		}
	}

	return nil
}

// buildPreservedCSS builds CSS that should be preserved in <style> tags
func (i *Inliner) buildPreservedCSS(stylesheet *css.Stylesheet) string {
	var preservedRules []string

	for _, rule := range stylesheet.Rules {
		shouldPreserve := false

		// Preserve media queries if configured
		if i.config.PreserveMediaQueries && i.isMediaQueryRule(rule.Selector) {
			shouldPreserve = true
		}

		// Preserve pseudo-selectors if configured
		if i.config.PreservePseudoSelectors && html.IsPseudoSelector(rule.Selector) {
			shouldPreserve = true
		}

		// Preserve rules that can't be inlined
		if i.isUninlinableRule(rule.Selector) {
			shouldPreserve = true
		}

		if shouldPreserve {
			cssRule := i.formatCSSRule(rule)
			preservedRules = append(preservedRules, cssRule)
		}
	}

	return strings.Join(preservedRules, "\n")
}

// isMediaQueryRule checks if a rule is inside a media query
func (i *Inliner) isMediaQueryRule(selector string) bool {
	// This is a simplified check - in practice, you'd need to track
	// the context during CSS parsing
	return strings.Contains(selector, "@media")
}

// isUninlinableRule checks if a rule cannot be inlined
func (i *Inliner) isUninlinableRule(selector string) bool {
	// Rules that must stay in <style> tags
	uninlinablePatterns := []string{
		"@keyframes",
		"@font-face",
		"@import",
		"@charset",
	}

	for _, pattern := range uninlinablePatterns {
		if strings.Contains(selector, pattern) {
			return true
		}
	}

	return false
}

// formatCSSRule converts a CSS rule back to CSS text
func (i *Inliner) formatCSSRule(rule css.Rule) string {
	var declarations []string

	for _, declaration := range rule.Declarations {
		value := declaration.Value
		if declaration.Important {
			value += " !important"
		}
		declarations = append(declarations, fmt.Sprintf("  %s: %s", declaration.Property, value))
	}

	return fmt.Sprintf("%s {\n%s;\n}", rule.Selector, strings.Join(declarations, ";\n"))
}

// removeElement removes an element from the document
func (i *Inliner) removeElement(element html.Node) error {
	return element.Remove()
}

// updateStyleTagContent updates the content of a style tag
func (i *Inliner) updateStyleTagContent(styleTag html.Node, content string) error {
	return styleTag.SetText(content)
}

// InlineCSS is a convenience function that inlines CSS with default configuration
func InlineCSS(htmlContent string) (string, error) {
	inliner := NewWithDefaults()
	return inliner.InlineString(htmlContent)
}

// InlineCSSWithConfig is a convenience function that inlines CSS with custom configuration
func InlineCSSWithConfig(htmlContent string, cfg config.Config) (string, error) {
	inliner := New(cfg)
	return inliner.InlineString(htmlContent)
}

// ValidateHTML validates HTML for email client compatibility
func (i *Inliner) ValidateHTML(htmlContent string) ([]ValidationIssue, error) {
	doc, err := i.htmlParser.Parse(htmlContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var issues []ValidationIssue

	// Check for problematic HTML elements
	issues = append(issues, i.validateHTMLStructure(doc)...)

	// Check for email-unsafe CSS
	issues = append(issues, i.validateEmbeddedCSS(doc)...)

	return issues, nil
}

// validateHTMLStructure checks for HTML structure issues
func (i *Inliner) validateHTMLStructure(doc html.Document) []ValidationIssue {
	var issues []ValidationIssue

	// Check for missing table structure in emails
	bodyElements, _ := doc.QuerySelectorAll("body *")
	hasTable := false

	for _, element := range bodyElements {
		if strings.ToLower(element.TagName()) == "table" {
			hasTable = true
			break
		}
	}

	if !hasTable {
		issues = append(issues, ValidationIssue{
			Type:     "structure",
			Severity: "warning",
			Message:  "Email should use table-based layout for better client compatibility",
			Element:  "body",
		})
	}

	return issues
}

// validateEmbeddedCSS checks for problematic CSS in style tags
func (i *Inliner) validateEmbeddedCSS(doc html.Document) []ValidationIssue {
	var issues []ValidationIssue

	styleTags, _ := doc.GetStyleTags()
	for _, styleTag := range styleTags {
		content := styleTag.Text()

		// Check for problematic CSS features
		if strings.Contains(content, "position:") && strings.Contains(content, "fixed") {
			issues = append(issues, ValidationIssue{
				Type:     "css",
				Severity: "error",
				Message:  "position: fixed is not supported in email clients",
				Element:  "style",
			})
		}
	}

	return issues
}

// ValidationIssue represents an email compatibility issue
type ValidationIssue struct {
	Type     string // "structure", "css", "attribute"
	Severity string // "error", "warning", "info"
	Message  string
	Element  string
	Property string // for CSS issues
}
