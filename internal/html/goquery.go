package html

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"inliner/internal/css"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// GoQueryDocument wraps goquery.Document to implement our Document interface
type GoQueryDocument struct {
	doc *goquery.Document
}

// GoQueryNode wraps goquery.Selection to implement our Node interface
// Export this type so it can be used in type assertions if needed
type GoQueryNode struct {
	selection *goquery.Selection
	doc       *GoQueryDocument
}

// GoQueryParser implements our Parser interface using goquery
type GoQueryParser struct{}

// NewParser creates a new GoQuery-based HTML parser
func NewParser() *GoQueryParser {
	return &GoQueryParser{}
}

// Parse parses HTML string into a Document
func (p *GoQueryParser) Parse(htmlStr string) (Document, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return &GoQueryDocument{doc: doc}, nil
}

// ParseFile parses HTML file into a Document
func (p *GoQueryParser) ParseFile(filename string) (Document, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(content)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML file: %w", err)
	}

	return &GoQueryDocument{doc: doc}, nil
}

// Document implementation

// Root returns the root HTML element
func (d *GoQueryDocument) Root() Node {
	selection := d.doc.Selection.Find("html").First()
	if selection.Length() == 0 {
		// If no html tag, use the document root
		selection = d.doc.Selection
	}
	return &GoQueryNode{selection: selection, doc: d}
}

// Head returns the head element
func (d *GoQueryDocument) Head() Node {
	selection := d.doc.Find("head").First()
	return &GoQueryNode{selection: selection, doc: d}
}

// Body returns the body element
func (d *GoQueryDocument) Body() Node {
	selection := d.doc.Find("body").First()
	return &GoQueryNode{selection: selection, doc: d}
}

// QuerySelector returns the first element matching the selector
func (d *GoQueryDocument) QuerySelector(selector string) (Node, error) {
	selection := d.doc.Find(selector).First()
	if selection.Length() == 0 {
		return nil, fmt.Errorf("no element found for selector: %s", selector)
	}
	return &GoQueryNode{selection: selection, doc: d}, nil
}

// QuerySelectorAll returns all elements matching the selector
func (d *GoQueryDocument) QuerySelectorAll(selector string) ([]Node, error) {
	selection := d.doc.Find(selector)
	nodes := make([]Node, selection.Length())

	selection.Each(func(i int, s *goquery.Selection) {
		nodes[i] = &GoQueryNode{selection: s, doc: d}
	})

	return nodes, nil
}

// GetStyleTags returns all <style> elements
func (d *GoQueryDocument) GetStyleTags() ([]Node, error) {
	return d.QuerySelectorAll("style")
}

// CreateStyleTag creates a new <style> element with content
func (d *GoQueryDocument) CreateStyleTag(content string) (Node, error) {
	// Use goquery to create and append a new style tag to head
	head := d.doc.Find("head").First()
	if head.Length() == 0 {
		return nil, fmt.Errorf("no head element found")
	}

	// Create style element HTML
	styleHTML := fmt.Sprintf("<style type=\"text/css\">%s</style>", content)
	head.AppendHtml(styleHTML)

	// Return the newly created style tag
	newStyle := head.Find("style").Last()
	return &GoQueryNode{selection: newStyle, doc: d}, nil
}

// HTML returns the complete HTML document as string
func (d *GoQueryDocument) HTML() (string, error) {
	html, err := d.doc.Html()
	if err != nil {
		return "", fmt.Errorf("failed to serialize HTML: %w", err)
	}
	return html, nil
}

// Node implementation

// TagName returns the element's tag name
func (n *GoQueryNode) TagName() string {
	if n.selection.Length() == 0 {
		return ""
	}
	return goquery.NodeName(n.selection)
}

// ID returns the element's ID attribute
func (n *GoQueryNode) ID() string {
	id, _ := n.selection.Attr("id")
	return id
}

// Classes returns the element's class list
func (n *GoQueryNode) Classes() []string {
	class, exists := n.selection.Attr("class")
	if !exists || class == "" {
		return []string{}
	}

	// Split by whitespace and filter empty strings
	classes := strings.Fields(class)
	return classes
}

// Attributes returns all attributes as a map
func (n *GoQueryNode) Attributes() map[string]string {
	attrs := make(map[string]string)

	if n.selection.Length() > 0 {
		node := n.selection.Get(0)
		for _, attr := range node.Attr {
			attrs[attr.Key] = attr.Val
		}
	}

	return attrs
}

// Text returns the text content
func (n *GoQueryNode) Text() string {
	return n.selection.Text()
}

// InnerHTML returns the inner HTML content
func (n *GoQueryNode) InnerHTML() string {
	html, _ := n.selection.Html()
	return html
}

// OuterHTML returns the outer HTML content
func (n *GoQueryNode) OuterHTML() string {
	if n.selection.Length() == 0 {
		return ""
	}

	// Get the underlying node and reconstruct outer HTML
	node := n.selection.Get(0)
	if node == nil {
		return ""
	}

	// Create a temporary buffer to render the node
	var buf strings.Builder
	err := html.Render(&buf, node)
	if err != nil {
		return ""
	}

	return buf.String()
}

// Parent returns the parent element
func (n *GoQueryNode) Parent() Node {
	parent := n.selection.Parent()
	if parent.Length() == 0 {
		return nil
	}
	return &GoQueryNode{selection: parent, doc: n.doc}
}

// Children returns all child elements
func (n *GoQueryNode) Children() []Node {
	children := n.selection.Children()
	nodes := make([]Node, children.Length())

	children.Each(func(i int, s *goquery.Selection) {
		nodes[i] = &GoQueryNode{selection: s, doc: n.doc}
	})

	return nodes
}

// NextSibling returns the next sibling element
func (n *GoQueryNode) NextSibling() Node {
	next := n.selection.Next()
	if next.Length() == 0 {
		return nil
	}
	return &GoQueryNode{selection: next, doc: n.doc}
}

// PrevSibling returns the previous sibling element
func (n *GoQueryNode) PrevSibling() Node {
	prev := n.selection.Prev()
	if prev.Length() == 0 {
		return nil
	}
	return &GoQueryNode{selection: prev, doc: n.doc}
}

// GetInlineStyle parses and returns the inline style attribute
func (n *GoQueryNode) GetInlineStyle() map[string]css.Declaration {
	styleAttr, exists := n.selection.Attr("style")
	if !exists || styleAttr == "" {
		return make(map[string]css.Declaration)
	}

	parser := css.NewParser()
	declarations, err := parser.ParseInlineStyle(styleAttr)
	if err != nil {
		return make(map[string]css.Declaration)
	}

	return declarations
}

// SetInlineStyle sets the complete inline style attribute
func (n *GoQueryNode) SetInlineStyle(styles map[string]css.Declaration) error {
	if n.selection.Length() == 0 {
		return fmt.Errorf("no element to set style on")
	}

	styleString := n.formatStyleString(styles)
	n.selection.SetAttr("style", styleString)
	return nil
}

// AddInlineStyle adds a single CSS property to inline styles
func (n *GoQueryNode) AddInlineStyle(property, value string, important bool) error {
	if n.selection.Length() == 0 {
		return fmt.Errorf("no element to add style to")
	}

	// Get existing styles
	existingStyles := n.GetInlineStyle()

	// Add new declaration
	existingStyles[property] = css.Declaration{
		Property:  property,
		Value:     value,
		Important: important,
	}

	// Set the updated styles
	return n.SetInlineStyle(existingStyles)
}

// formatStyleString converts declarations map to CSS string
func (n *GoQueryNode) formatStyleString(styles map[string]css.Declaration) string {
	if len(styles) == 0 {
		return ""
	}

	var parts []string
	for _, declaration := range styles {
		value := declaration.Value
		if declaration.Important {
			value += " !important"
		}
		parts = append(parts, fmt.Sprintf("%s: %s", declaration.Property, value))
	}

	return strings.Join(parts, "; ")
}

// Matches checks if the element matches a CSS selector
func (n *GoQueryNode) Matches(selector string) (bool, error) {
	if n.selection.Length() == 0 {
		return false, nil
	}

	// Use goquery's Is() method for selector matching
	return n.selection.Is(selector), nil
}

// SetAttribute sets an attribute on the element
func (n *GoQueryNode) SetAttribute(name, value string) error {
	if n.selection.Length() == 0 {
		return fmt.Errorf("no element to set attribute on")
	}

	n.selection.SetAttr(name, value)
	return nil
}

// RemoveAttribute removes an attribute from the element
func (n *GoQueryNode) RemoveAttribute(name string) error {
	if n.selection.Length() == 0 {
		return fmt.Errorf("no element to remove attribute from")
	}

	n.selection.RemoveAttr(name)
	return nil
}

// Helper functions for CSS selector matching

// SelectorComplexity estimates the complexity of a CSS selector
func SelectorComplexity(selector string) int {
	complexity := 0

	// Count different selector types
	complexity += strings.Count(selector, "#") // IDs
	complexity += strings.Count(selector, ".") // Classes
	complexity += strings.Count(selector, "[") // Attributes
	complexity += strings.Count(selector, ":") // Pseudo-classes
	complexity += strings.Count(selector, " ") // Descendant combinators
	complexity += strings.Count(selector, ">") // Child combinators
	complexity += strings.Count(selector, "+") // Adjacent sibling
	complexity += strings.Count(selector, "~") // General sibling

	return complexity
}

// NormalizeSelector normalizes a CSS selector for consistent matching
func NormalizeSelector(selector string) string {
	// Remove extra whitespace
	selector = regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(selector), " ")

	// Normalize combinators
	selector = regexp.MustCompile(`\s*>\s*`).ReplaceAllString(selector, " > ")
	selector = regexp.MustCompile(`\s*\+\s*`).ReplaceAllString(selector, " + ")
	selector = regexp.MustCompile(`\s*~\s*`).ReplaceAllString(selector, " ~ ")

	return selector
}

// IsPseudoSelector checks if a selector contains pseudo-classes or pseudo-elements
func IsPseudoSelector(selector string) bool {
	return strings.Contains(selector, ":") &&
		(strings.Contains(selector, ":hover") ||
			strings.Contains(selector, ":focus") ||
			strings.Contains(selector, ":active") ||
			strings.Contains(selector, ":visited") ||
			strings.Contains(selector, "::before") ||
			strings.Contains(selector, "::after"))
}

// IsMediaQuerySelector checks if a selector is inside a media query
func IsMediaQuerySelector(fullCSS, selector string) bool {
	// Simple check - in real implementation, would need proper CSS parsing
	mediaRegex := regexp.MustCompile(`@media[^{]*\{[^}]*` + regexp.QuoteMeta(selector) + `[^}]*\}`)
	return mediaRegex.MatchString(fullCSS)
}
