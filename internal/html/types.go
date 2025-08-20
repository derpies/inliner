package html

import "inliner/internal/css"

// Node represents an HTML element in the DOM tree
// This interface can be implemented by any HTML parsing library
type Node interface {
	// Core node information
	TagName() string
	ID() string
	Classes() []string
	Attributes() map[string]string

	// Content access
	Text() string
	InnerHTML() string
	OuterHTML() string

	// Tree navigation
	Parent() Node
	Children() []Node
	NextSibling() Node
	PrevSibling() Node

	// Style manipulation
	GetInlineStyle() map[string]css.Declaration
	SetInlineStyle(styles map[string]css.Declaration) error
	AddInlineStyle(property, value string, important bool) error

	// Selector matching support
	Matches(selector string) (bool, error)

	// Modification
	SetAttribute(name, value string) error
	RemoveAttribute(name string) error
	Remove() error
	SetText(content string) error
	SetHTML(content string) error
}

// Document represents the complete HTML document
type Document interface {
	// Root access
	Root() Node
	Head() Node
	Body() Node

	// Element selection
	QuerySelector(selector string) (Node, error)
	QuerySelectorAll(selector string) ([]Node, error)

	// Style tag management
	GetStyleTags() ([]Node, error)
	CreateStyleTag(content string) (Node, error)

	// Serialization
	HTML() (string, error)
}

// Parser handles parsing HTML documents
type Parser interface {
	Parse(html string) (Document, error)
	ParseFile(filename string) (Document, error)
}
