package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"inliner/internal/config"
	"inliner/pkg/inliner"
)

var (
	// Input/Output flags
	inputFile  = flag.String("input", "", "Input HTML file path")
	outputFile = flag.String("output", "", "Output HTML file path (default: stdout)")
	inputDir   = flag.String("input-dir", "", "Process all HTML files in directory")
	outputDir  = flag.String("output-dir", "", "Output directory for batch processing")

	// Configuration flags
	target             = flag.String("target", "generic", "Target email client (outlook, gmail, apple_mail, generic)")
	preserveMedia      = flag.Bool("preserve-media", true, "Preserve @media queries in <style> tags")
	preservePseudo     = flag.Bool("preserve-pseudo", true, "Preserve pseudo-selectors (:hover, :focus, etc.)")
	removeStyleTags    = flag.Bool("remove-style-tags", false, "Remove <style> tags after inlining")
	stripUnused        = flag.Bool("strip-unused", true, "Remove CSS rules that don't match any elements")
	emailOptimizations = flag.Bool("email-optimizations", true, "Apply email client optimizations")
	preserveWhitespace = flag.Bool("preserve-whitespace", true, "Preserve HTML formatting")

	// Output control flags
	verbose = flag.Bool("verbose", false, "Verbose output with processing statistics")
	quiet   = flag.Bool("quiet", false, "Suppress all output except errors")
	stats   = flag.Bool("stats", false, "Show processing statistics")

	// Validation flags
	validate     = flag.Bool("validate", false, "Validate HTML for email compatibility (no inlining)")
	showWarnings = flag.Bool("warnings", true, "Show compatibility warnings")

	// Performance flags
	benchmark = flag.Bool("benchmark", false, "Show processing time and performance metrics")
)

func main() {
	flag.Parse()

	// Validate command line arguments
	if err := validateArgs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	// Create configuration from flags
	cfg := buildConfig()

	// Create inliner instance
	inlinerEngine := inliner.New(cfg)

	var err error
	startTime := time.Now()

	// Route to appropriate processing mode
	switch {
	case *validate:
		err = runValidation(inlinerEngine)
	case *inputDir != "":
		err = runBatchProcessing(inlinerEngine)
	case *inputFile != "":
		err = runSingleFile(inlinerEngine)
	default:
		err = runStdin(inlinerEngine)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Show performance metrics if requested
	if *benchmark {
		duration := time.Since(startTime)
		fmt.Fprintf(os.Stderr, "Processing completed in %v\n", duration)
	}
}

// validateArgs validates command line arguments
func validateArgs() error {
	if *inputFile != "" && *inputDir != "" {
		return fmt.Errorf("cannot specify both -input and -input-dir")
	}

	if *inputDir != "" && *outputDir == "" {
		return fmt.Errorf("-output-dir required when using -input-dir")
	}

	if *quiet && *verbose {
		return fmt.Errorf("cannot specify both -quiet and -verbose")
	}

	// Validate target email client
	validTargets := []string{"outlook", "gmail", "apple_mail", "outlook_online", "generic"}
	targetValid := false
	for _, valid := range validTargets {
		if *target == valid {
			targetValid = true
			break
		}
	}
	if !targetValid {
		return fmt.Errorf("invalid target client: %s (valid: %s)", *target, strings.Join(validTargets, ", "))
	}

	return nil
}

// buildConfig creates configuration from command line flags
func buildConfig() config.Config {
	return config.Config{
		PreserveMediaQueries:     *preserveMedia,
		PreservePseudoSelectors:  *preservePseudo,
		RemoveStyleTags:          *removeStyleTags,
		StripUnusedCSS:           *stripUnused,
		EmailClientOptimizations: *emailOptimizations,
		PreserveWhitespace:       *preserveWhitespace,
		TargetEmailClient:        *target,
	}
}

// runSingleFile processes a single input file
func runSingleFile(inlinerEngine *inliner.Inliner) error {
	// Read input file
	inputContent, err := os.ReadFile(*inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file %s: %w", *inputFile, err)
	}

	// Process the HTML
	result, err := inlinerEngine.Inline(string(inputContent))
	if err != nil {
		return fmt.Errorf("failed to inline CSS: %w", err)
	}

	// Write output
	if err := writeOutput(result.HTML, *outputFile); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	// Show statistics if requested
	if *stats || *verbose {
		showProcessingStats(result, *inputFile)
	}

	// Show warnings if requested
	if *showWarnings && len(result.Warnings) > 0 {
		showWarnings(result.Warnings)
	}

	return nil
}

// runBatchProcessing processes all HTML files in a directory
func runBatchProcessing(inlinerEngine *inliner.Inliner) error {
	// Find all HTML files in input directory
	htmlFiles, err := findHTMLFiles(*inputDir)
	if err != nil {
		return fmt.Errorf("failed to find HTML files: %w", err)
	}

	if len(htmlFiles) == 0 {
		return fmt.Errorf("no HTML files found in directory: %s", *inputDir)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Process each file
	var totalStats inliner.ProcessingStats
	var totalWarnings []inliner.ValidationWarning

	for i, inputPath := range htmlFiles {
		if *verbose {
			fmt.Fprintf(os.Stderr, "Processing %d/%d: %s\n", i+1, len(htmlFiles), inputPath)
		}

		// Read input file
		inputContent, err := os.ReadFile(inputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", inputPath, err)
			continue
		}

		// Process the HTML
		result, err := inlinerEngine.Inline(string(inputContent))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to process %s: %v\n", inputPath, err)
			continue
		}

		// Generate output path
		relPath, _ := filepath.Rel(*inputDir, inputPath)
		outputPath := filepath.Join(*outputDir, relPath)

		// Create output subdirectory if needed
		outputSubdir := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputSubdir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create output directory %s: %v\n", outputSubdir, err)
			continue
		}

		// Write output file
		if err := writeOutput(result.HTML, outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", outputPath, err)
			continue
		}

		// Accumulate statistics
		totalStats.CSSRulesParsed += result.ProcessingStats.CSSRulesParsed
		totalStats.HTMLElementsProcessed += result.ProcessingStats.HTMLElementsProcessed
		totalStats.SelectorsMatched += result.ProcessingStats.SelectorsMatched
		totalStats.ProcessingTimeMs += result.ProcessingStats.ProcessingTimeMs
		totalWarnings = append(totalWarnings, result.Warnings...)
	}

	// Show batch statistics
	if *stats || *verbose {
		fmt.Fprintf(os.Stderr, "\nBatch Processing Summary:\n")
		fmt.Fprintf(os.Stderr, "Files processed: %d\n", len(htmlFiles))
		fmt.Fprintf(os.Stderr, "CSS rules parsed: %d\n", totalStats.CSSRulesParsed)
		fmt.Fprintf(os.Stderr, "HTML elements processed: %d\n", totalStats.HTMLElementsProcessed)
		fmt.Fprintf(os.Stderr, "Selectors matched: %d\n", totalStats.SelectorsMatched)
		fmt.Fprintf(os.Stderr, "Total processing time: %dms\n", totalStats.ProcessingTimeMs)

		if len(totalWarnings) > 0 {
			fmt.Fprintf(os.Stderr, "Total warnings: %d\n", len(totalWarnings))
		}
	}

	return nil
}

// runStdin processes HTML from stdin and outputs to stdout
func runStdin(inlinerEngine *inliner.Inliner) error {
	// Read from stdin
	inputContent, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read from stdin: %w", err)
	}

	// Process the HTML
	result, err := inlinerEngine.Inline(string(inputContent))
	if err != nil {
		return fmt.Errorf("failed to inline CSS: %w", err)
	}

	// Write to stdout or specified output file
	if err := writeOutput(result.HTML, *outputFile); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	// Show statistics to stderr if requested (so they don't interfere with HTML output)
	if *stats || *verbose {
		showProcessingStats(result, "<stdin>")
	}

	return nil
}

// runValidation validates HTML without inlining
func runValidation(inlinerEngine *inliner.Inliner) error {
	var inputContent []byte
	var err error
	var filename string

	if *inputFile != "" {
		inputContent, err = os.ReadFile(*inputFile)
		filename = *inputFile
	} else {
		inputContent, err = io.ReadAll(os.Stdin)
		filename = "<stdin>"
	}

	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	// Validate the HTML
	issues, err := inlinerEngine.ValidateHTML(string(inputContent))
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Show validation results
	if len(issues) == 0 {
		if !*quiet {
			fmt.Printf("✓ %s: No email compatibility issues found\n", filename)
		}
	} else {
		fmt.Printf("✗ %s: Found %d email compatibility issues:\n", filename, len(issues))
		for _, issue := range issues {
			severity := strings.ToUpper(issue.Severity)
			fmt.Printf("  [%s] %s: %s\n", severity, issue.Element, issue.Message)
			if issue.Property != "" {
				fmt.Printf("         Property: %s\n", issue.Property)
			}
		}
	}

	return nil
}

// writeOutput writes content to a file or stdout
func writeOutput(content, filename string) error {
	if filename == "" {
		// Write to stdout
		_, err := fmt.Print(content)
		return err
	}

	// Write to file
	return os.WriteFile(filename, []byte(content), 0644)
}

// findHTMLFiles finds all HTML files in a directory
func findHTMLFiles(dir string) ([]string, error) {
	var htmlFiles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".html" || ext == ".htm" {
				htmlFiles = append(htmlFiles, path)
			}
		}

		return nil
	})

	return htmlFiles, err
}

// showProcessingStats displays processing statistics
func showProcessingStats(result *inliner.InlineResult, filename string) {
	fmt.Fprintf(os.Stderr, "\nProcessing Statistics for %s:\n", filename)
	fmt.Fprintf(os.Stderr, "  Inlined styles: %d\n", result.InlinedStyles)
	fmt.Fprintf(os.Stderr, "  Preserved rules: %d\n", result.PreservedRules)
	fmt.Fprintf(os.Stderr, "  CSS rules parsed: %d\n", result.ProcessingStats.CSSRulesParsed)
	fmt.Fprintf(os.Stderr, "  HTML elements processed: %d\n", result.ProcessingStats.HTMLElementsProcessed)
	fmt.Fprintf(os.Stderr, "  Selectors matched: %d\n", result.ProcessingStats.SelectorsMatched)
	fmt.Fprintf(os.Stderr, "  Processing time: %dms\n", result.ProcessingStats.ProcessingTimeMs)
}

// showWarnings displays compatibility warnings
func showWarnings(warnings []inliner.ValidationWarning) {
	if len(warnings) == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "\nCompatibility Warnings:\n")
	for _, warning := range warnings {
		severity := strings.ToUpper(warning.Severity)
		fmt.Fprintf(os.Stderr, "  [%s] %s: %s (%s)\n", severity, warning.Property, warning.Message, warning.Value)
	}
}
