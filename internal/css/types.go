package css

import (
	"fmt"
)

// Specificity represents CSS specificity with individual components
// Following CSS specification: inline, IDs, classes/attributes/pseudo-classes, elements/pseudo-elements
type Specificity struct {
	Inline    int  // style="" attribute (always 1000 when present)
	IDs       int  // #id selectors
	Classes   int  // .class, [attr], :pseudo-class
	Elements  int  // element, ::pseudo-element
	Important bool // !important flag
}

// Compare returns -1 if s < other, 0 if equal, 1 if s > other
// Important declarations always win regardless of specificity
func (s Specificity) Compare(other Specificity) int {
	// !important always wins
	if s.Important && !other.Important {
		return 1
	}
	if !s.Important && other.Important {
		return -1
	}

	// Compare specificity components in order
	if s.Inline != other.Inline {
		if s.Inline > other.Inline {
			return 1
		}
		return -1
	}
	if s.IDs != other.IDs {
		if s.IDs > other.IDs {
			return 1
		}
		return -1
	}
	if s.Classes != other.Classes {
		if s.Classes > other.Classes {
			return 1
		}
		return -1
	}
	if s.Elements != other.Elements {
		if s.Elements > other.Elements {
			return 1
		}
		return -1
	}

	return 0 // Equal specificity
}

func (s Specificity) String() string {
	important := ""
	if s.Important {
		important = " !important"
	}
	return fmt.Sprintf("(%d,%d,%d,%d)%s", s.Inline, s.IDs, s.Classes, s.Elements, important)
}

// Rule represents a single CSS rule with its selector and declarations
type Rule struct {
	Selector     string                 // Original selector text
	Specificity  Specificity            // Calculated specificity
	Declarations map[string]Declaration // property -> declaration mapping
	SourceOrder  int                    // Order in original CSS (for tie-breaking)
}

// Declaration represents a single CSS property declaration
type Declaration struct {
	Property  string // CSS property name (normalized)
	Value     string // CSS property value
	Important bool   // !important flag
}

// Stylesheet represents the complete parsed CSS with all rules
type Stylesheet struct {
	Rules []Rule // All CSS rules in source order
}

// MatchResult represents the result of matching CSS rules against an HTML element
type MatchResult struct {
	Rule         *Rule                  // The matching CSS rule
	Specificity  Specificity            // Effective specificity for this match
	Declarations map[string]Declaration // Declarations that should be applied
}
