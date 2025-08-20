package resolver

import (
	"fmt"
	"sort"
	"strings"

	"inliner/internal/config"
	"inliner/internal/css"
	"inliner/internal/html"
)

// Resolver handles CSS cascade resolution and computes final styles for HTML elements
type Resolver struct {
	stylesheet *css.Stylesheet
	config     config.Config
	parser     *css.Parser
}

// New creates a new style resolver
func New(stylesheet *css.Stylesheet, cfg config.Config) *Resolver {
	return &Resolver{
		stylesheet: stylesheet,
		config:     cfg,
		parser:     css.NewParser(),
	}
}

// ResolveStyles computes the final styles for an HTML element following CSS cascade rules
// Returns a map of property -> declaration with all cascade rules applied
func (r *Resolver) ResolveStyles(node html.Node) (map[string]css.Declaration, error) {
	// Step 1: Find all CSS rules that match this element
	matchingRules, err := r.findMatchingRules(node)
	if err != nil {
		return nil, fmt.Errorf("failed to find matching rules: %w", err)
	}

	// Step 2: Get existing inline styles
	inlineStyles := node.GetInlineStyle()

	// Step 3: Apply CSS cascade to determine winning declarations
	finalStyles := r.applyCascade(matchingRules, inlineStyles)

	// Step 4: Filter styles based on email client compatibility
	if r.config.EmailClientOptimizations {
		finalStyles = r.filterEmailSafeStyles(finalStyles)
	}

	return finalStyles, nil
}

// findMatchingRules finds all CSS rules that match the given HTML element
func (r *Resolver) findMatchingRules(node html.Node) ([]css.MatchResult, error) {
	var matches []css.MatchResult

	for _, rule := range r.stylesheet.Rules {
		// Check if the selector matches this element
		isMatch, err := node.Matches(rule.Selector)
		if err != nil {
			// Log selector matching error but continue
			continue
		}

		if isMatch {
			matches = append(matches, css.MatchResult{
				Rule:         &rule,
				Specificity:  rule.Specificity,
				Declarations: rule.Declarations,
			})
		}
	}

	return matches, nil
}

// applyCascade applies CSS cascade rules to determine which declarations win
// CSS Cascade order (lowest to highest priority):
// 1. User agent declarations
// 2. User declarations
// 3. Author declarations (our CSS)
// 4. Author !important declarations
// 5. User !important declarations
// 6. User agent !important declarations
func (r *Resolver) applyCascade(matches []css.MatchResult, inlineStyles map[string]css.Declaration) map[string]css.Declaration {
	// Map to store the winning declaration for each property
	winningDeclarations := make(map[string]css.Declaration)

	// Map to track the winning specificity and source order for each property
	winningSpecs := make(map[string]cascadeEntry)

	// Step 1: Process stylesheet rules
	for _, match := range matches {
		for property, declaration := range match.Declarations {
			entry := cascadeEntry{
				specificity: match.Specificity,
				sourceOrder: match.Rule.SourceOrder,
				important:   declaration.Important,
				isInline:    false,
			}

			if r.shouldReplace(property, entry, winningSpecs[property]) {
				winningDeclarations[property] = declaration
				winningSpecs[property] = entry
			}
		}
	}

	// Step 2: Process inline styles (they have specificity 1000,0,0,0)
	for property, declaration := range inlineStyles {
		entry := cascadeEntry{
			specificity: css.SpecificityFromInline(declaration.Important),
			sourceOrder: 999999, // Inline styles come last in source order
			important:   declaration.Important,
			isInline:    true,
		}

		if r.shouldReplace(property, entry, winningSpecs[property]) {
			winningDeclarations[property] = declaration
			winningSpecs[property] = entry
		}
	}

	return winningDeclarations
}

// cascadeEntry tracks the cascade information for a declaration
type cascadeEntry struct {
	specificity css.Specificity
	sourceOrder int
	important   bool
	isInline    bool
}

// shouldReplace determines if a new declaration should replace the existing winning declaration
func (r *Resolver) shouldReplace(property string, newEntry, existingEntry cascadeEntry) bool {
	// If no existing entry, new one wins
	if existingEntry.specificity.Inline == 0 && existingEntry.specificity.IDs == 0 &&
		existingEntry.specificity.Classes == 0 && existingEntry.specificity.Elements == 0 {
		return true
	}

	// Compare by cascade rules:
	// 1. !important declarations always beat non-!important
	if newEntry.important && !existingEntry.important {
		return true
	}
	if !newEntry.important && existingEntry.important {
		return false
	}

	// 2. Higher specificity wins
	specificityComparison := newEntry.specificity.Compare(existingEntry.specificity)
	if specificityComparison > 0 {
		return true
	}
	if specificityComparison < 0 {
		return false
	}

	// 3. If specificity is equal, later source order wins
	return newEntry.sourceOrder >= existingEntry.sourceOrder
}

// filterEmailSafeStyles removes CSS properties that don't work well in email clients
func (r *Resolver) filterEmailSafeStyles(styles map[string]css.Declaration) map[string]css.Declaration {
	if !r.config.EmailClientOptimizations {
		return styles
	}

	// Get compatibility profile for target email client
	compatibility := config.GetCompatibilityProfile(r.config.TargetEmailClient)

	filtered := make(map[string]css.Declaration)

	for property, declaration := range styles {
		// Always keep email-safe properties
		if css.IsEmailSafeProperty(property) {
			filtered[property] = declaration
			continue
		}

		// Handle email client specific rules
		switch property {
		case "position":
			// Most email clients don't support positioning
			if !compatibility.RequiresInlineStyles {
				filtered[property] = declaration
			}

		case "float":
			// Float works in most clients but can be problematic
			if r.config.TargetEmailClient != "outlook" {
				filtered[property] = declaration
			}

		case "display":
			// Display property support varies widely
			if declaration.Value == "block" || declaration.Value == "inline" ||
				declaration.Value == "table" || declaration.Value == "table-cell" {
				filtered[property] = declaration
			}

		default:
			// For unknown properties, be conservative
			if !compatibility.RequiresInlineStyles {
				filtered[property] = declaration
			}
		}
	}

	return filtered
}

// MergeStyles merges new styles with existing inline styles
// New styles take precedence unless existing style has !important and new doesn't
func (r *Resolver) MergeStyles(existing, newStyles map[string]css.Declaration) map[string]css.Declaration {
	merged := make(map[string]css.Declaration)

	// Start with existing styles
	for property, declaration := range existing {
		merged[property] = declaration
	}

	// Add new styles, respecting !important
	for property, newDeclaration := range newStyles {
		existingDeclaration, exists := merged[property]

		if !exists {
			// No existing declaration, add new one
			merged[property] = newDeclaration
		} else {
			// Both exist, apply cascade rules
			if r.shouldReplaceDeclaration(existingDeclaration, newDeclaration) {
				merged[property] = newDeclaration
			}
		}
	}

	return merged
}

// shouldReplaceDeclaration determines if new declaration should replace existing
func (r *Resolver) shouldReplaceDeclaration(existing, new css.Declaration) bool {
	// !important wins over non-!important
	if new.Important && !existing.Important {
		return true
	}
	if !new.Important && existing.Important {
		return false
	}

	// If both have same !important status, new one wins (later in cascade)
	return true
}

// StylesString converts a styles map to a CSS string for inline styles
func (r *Resolver) StylesString(styles map[string]css.Declaration) string {
	if len(styles) == 0 {
		return ""
	}

	var parts []string

	// Sort properties for consistent output
	properties := make([]string, 0, len(styles))
	for property := range styles {
		properties = append(properties, property)
	}
	sort.Strings(properties)

	for _, property := range properties {
		declaration := styles[property]
		value := declaration.Value

		if declaration.Important {
			value += " !important"
		}

		parts = append(parts, fmt.Sprintf("%s: %s", property, value))
	}

	return strings.Join(parts, "; ")
}

// GetConflictingProperties returns properties that have different values
// between existing and new styles - useful for debugging cascade issues
func (r *Resolver) GetConflictingProperties(existing, newStyles map[string]css.Declaration) map[string]ConflictInfo {
	conflicts := make(map[string]ConflictInfo)

	for property, newDecl := range newStyles {
		if existingDecl, exists := existing[property]; exists {
			if existingDecl.Value != newDecl.Value || existingDecl.Important != newDecl.Important {
				conflicts[property] = ConflictInfo{
					ExistingValue:     existingDecl.Value,
					ExistingImportant: existingDecl.Important,
					NewValue:          newDecl.Value,
					NewImportant:      newDecl.Important,
					Winner:            r.shouldReplaceDeclaration(existingDecl, newDecl),
				}
			}
		}
	}

	return conflicts
}

// ConflictInfo describes a property conflict between existing and new styles
type ConflictInfo struct {
	ExistingValue     string
	ExistingImportant bool
	NewValue          string
	NewImportant      bool
	Winner            bool // true if new declaration wins
}

// ValidateStyles checks if the computed styles are valid for email clients
func (r *Resolver) ValidateStyles(styles map[string]css.Declaration) []ValidationWarning {
	var warnings []ValidationWarning

	compatibility := config.GetCompatibilityProfile(r.config.TargetEmailClient)

	for property, declaration := range styles {
		// Check for problematic property values
		switch property {
		case "background-image":
			if strings.Contains(declaration.Value, "url(") && r.config.TargetEmailClient == "outlook" {
				warnings = append(warnings, ValidationWarning{
					Property: property,
					Value:    declaration.Value,
					Message:  "Background images may not render in Outlook desktop",
					Severity: "warning",
				})
			}

		case "width", "height":
			if strings.Contains(declaration.Value, "vw") || strings.Contains(declaration.Value, "vh") {
				warnings = append(warnings, ValidationWarning{
					Property: property,
					Value:    declaration.Value,
					Message:  "Viewport units not supported in email clients",
					Severity: "error",
				})
			}

		case "position":
			if declaration.Value != "static" && compatibility.RequiresInlineStyles {
				warnings = append(warnings, ValidationWarning{
					Property: property,
					Value:    declaration.Value,
					Message:  "Positioning not supported in this email client",
					Severity: "warning",
				})
			}
		}

		// Check for email-unsafe properties
		if !css.IsEmailSafeProperty(property) {
			warnings = append(warnings, ValidationWarning{
				Property: property,
				Value:    declaration.Value,
				Message:  "Property may not be supported across all email clients",
				Severity: "info",
			})
		}
	}

	return warnings
}

// ValidationWarning represents a potential issue with computed styles
type ValidationWarning struct {
	Property string
	Value    string
	Message  string
	Severity string // "error", "warning", "info"
}
