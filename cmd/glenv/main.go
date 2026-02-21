package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/jessevdk/go-flags"
	"github.com/ohmylock/glenv/pkg/classifier"
	"github.com/ohmylock/glenv/pkg/config"
	"github.com/ohmylock/glenv/pkg/envfile"
	"github.com/ohmylock/glenv/pkg/gitlab"
	glsync "github.com/ohmylock/glenv/pkg/sync"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

// appCtx is the package-level context used by go-flags commands (Execute lacks ctx).
var appCtx context.Context

// Color variables for output formatting.
var (
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	cyan   = color.New(color.FgCyan)
	red    = color.New(color.FgRed)
	gray   = color.New(color.FgHiBlack)
)

// GlobalOptions holds flags shared across all commands.
type GlobalOptions struct {
	Config    string  `short:"c" long:"config" description:"Path to .glenv.yml config file"`
	Token     string  `long:"token" env:"GITLAB_TOKEN" description:"GitLab private token"`
	Project   string  `long:"project" env:"GITLAB_PROJECT_ID" description:"GitLab project ID"`
	URL       string  `long:"url" env:"GITLAB_URL" description:"GitLab base URL"`
	DryRun    bool    `short:"n" long:"dry-run" description:"Print planned changes without applying them"`
	NoColor   bool    `long:"no-color" description:"Disable colored output"`
	Workers   int     `short:"w" long:"workers" description:"Number of concurrent workers"`
	RateLimit float64 `long:"rate-limit" description:"Max API requests per second"`
}

// VersionCommand prints the build version.
type VersionCommand struct{}

func (cmd *VersionCommand) Execute(args []string) error {
	fmt.Printf("glenv version %s\n", version)
	return nil
}

// SyncCommand pushes local .env variables to GitLab.
type SyncCommand struct {
	File          string `short:"f" long:"file" description:"Path to .env file (resolves from config or defaults to .env)"`
	Environment   string `short:"e" long:"environment" description:"GitLab environment scope" default:"*"`
	All           bool   `short:"a" long:"all" description:"Sync all environments defined in config"`
	DeleteMissing bool   `long:"delete-missing" description:"Delete remote variables not present in .env file"`
	NoAutoClassify bool  `long:"no-auto-classify" description:"Disable automatic variable classification"`
	Force         bool   `long:"force" description:"Skip confirmation prompt"`
	global        *GlobalOptions
}

func (cmd *SyncCommand) Execute(args []string) error {
	setupColor(cmd.global.NoColor)
	printHeader()
	cfg, client, err := buildClientFromGlobal(cmd.global)
	if err != nil {
		return err
	}

	// --all: sync each environment defined in config file.
	if cmd.All {
		if len(cfg.Environments) == 0 {
			return fmt.Errorf("--all requires environments to be defined in config file")
		}
		envNames := make([]string, 0, len(cfg.Environments))
		for name := range cfg.Environments {
			envNames = append(envNames, name)
		}
		sort.Strings(envNames)

		var errs []error
		for _, envName := range envNames {
			envFile := resolveEnvFile(cmd.File, envName, cfg)
			fmt.Printf("\n=== Syncing environment: %s (file: %s) ===\n", envName, envFile)
			if err := cmd.syncOne(cfg, client, envFile, envName); err != nil {
				red.Printf("error syncing %s: %v\n", envName, err)
				errs = append(errs, fmt.Errorf("%s: %w", envName, err))
			}
		}
		return errors.Join(errs...)
	}

	return cmd.syncOne(cfg, client, resolveEnvFile(cmd.File, cmd.Environment, cfg), cmd.Environment)
}

// syncOne performs a single sync of envFile to the given environment scope.
func (cmd *SyncCommand) syncOne(cfg *config.Config, client *gitlab.Client, envFile, envScope string) error {
	parsed, err := envfile.ParseFile(envFile)
	if err != nil {
		return fmt.Errorf("parse %s: %w", envFile, err)
	}

	cl := buildClassifier(cfg, cmd.NoAutoClassify)
	opts := glsync.Options{
		Workers:       resolveWorkers(cmd.global, cfg),
		DryRun:        cmd.global.DryRun,
		DeleteMissing: cmd.DeleteMissing,
	}
	engine := glsync.NewEngine(client, cl, opts, cfg.GitLab.ProjectID)

	remote, err := client.ListVariables(appCtx, cfg.GitLab.ProjectID, gitlab.ListOptions{EnvironmentScope: envScope})
	if err != nil {
		return fmt.Errorf("list remote variables: %w", err)
	}

	diff := engine.Diff(appCtx, parsed.Variables, remote, envScope)

	printDiff(diff)
	if cmd.global.DryRun {
		printDiffSummary(diff)
		return nil
	}
	// Only prompt when --delete-missing would actually delete variables.
	if cmd.DeleteMissing && !cmd.Force {
		deleteCount := 0
		for _, ch := range diff.Changes {
			if ch.Kind == glsync.ChangeDelete {
				deleteCount++
			}
		}
		if deleteCount > 0 {
			if !confirm(fmt.Sprintf("Delete %d variable(s)?", deleteCount)) {
				fmt.Println("Aborted.")
				return nil
			}
		}
	}

	fmt.Printf("\nSyncing: %s → project %s (%s)\n", envFile, cfg.GitLab.ProjectID, envScope)
	fmt.Println(separator)
	fmt.Println()
	report := engine.ApplyWithCallback(appCtx, diff, func(r glsync.Result) {
		printResult(r)
	})

	printSyncReport(report)
	if report.Failed > 0 {
		return fmt.Errorf("%d variable(s) failed to sync", report.Failed)
	}
	return nil
}

// DiffCommand shows what would change without applying.
type DiffCommand struct {
	File          string `short:"f" long:"file" description:"Path to .env file (resolves from config or defaults to .env)"`
	Environment   string `short:"e" long:"environment" description:"GitLab environment scope" default:"*"`
	DeleteMissing bool   `long:"delete-missing" description:"Show variables that would be deleted"`
	global        *GlobalOptions
}

func (cmd *DiffCommand) Execute(args []string) error {
	setupColor(cmd.global.NoColor)
	cfg, client, err := buildClientFromGlobal(cmd.global)
	if err != nil {
		return err
	}

	envFile := resolveEnvFile(cmd.File, cmd.Environment, cfg)
	parsed, err := envfile.ParseFile(envFile)
	if err != nil {
		return fmt.Errorf("parse %s: %w", envFile, err)
	}

	cl := buildClassifier(cfg, false)
	opts := glsync.Options{
		Workers:       resolveWorkers(cmd.global, cfg),
		DeleteMissing: cmd.DeleteMissing,
	}
	engine := glsync.NewEngine(client, cl, opts, cfg.GitLab.ProjectID)

	remote, err := client.ListVariables(appCtx, cfg.GitLab.ProjectID, gitlab.ListOptions{EnvironmentScope: cmd.Environment})
	if err != nil {
		return fmt.Errorf("list remote variables: %w", err)
	}

	diff := engine.Diff(appCtx, parsed.Variables, remote, cmd.Environment)
	printDiff(diff)
	printDiffSummary(diff)
	return nil
}

// ListCommand fetches and displays all remote variables.
type ListCommand struct {
	Environment string `short:"e" long:"environment" description:"Filter by environment scope"`
	global      *GlobalOptions
}

func (cmd *ListCommand) Execute(args []string) error {
	setupColor(cmd.global.NoColor)
	cfg, client, err := buildClientFromGlobal(cmd.global)
	if err != nil {
		return err
	}

	vars, err := client.ListVariables(appCtx, cfg.GitLab.ProjectID, gitlab.ListOptions{EnvironmentScope: cmd.Environment})
	if err != nil {
		return fmt.Errorf("list variables: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tTYPE\tSCOPE\tMASKED\tPROTECTED")
	for _, v := range vars {
		masked := "-"
		if v.Masked {
			masked = "yes"
		}
		protected := "-"
		if v.Protected {
			protected = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Key, v.VariableType, v.EnvironmentScope, masked, protected)
	}
	w.Flush()
	fmt.Printf("\nTotal: %d variables\n", len(vars))
	return nil
}

// ExportCommand writes remote variables as KEY=VALUE lines.
type ExportCommand struct {
	Environment string `short:"e" long:"environment" description:"Filter by environment scope"`
	Output      string `short:"o" long:"output" description:"Output file path (default: stdout)"`
	global      *GlobalOptions
}

func (cmd *ExportCommand) Execute(args []string) error {
	setupColor(cmd.global.NoColor)
	cfg, client, err := buildClientFromGlobal(cmd.global)
	if err != nil {
		return err
	}

	vars, err := client.ListVariables(appCtx, cfg.GitLab.ProjectID, gitlab.ListOptions{EnvironmentScope: cmd.Environment})
	if err != nil {
		return fmt.Errorf("list variables: %w", err)
	}

	out := io.Writer(os.Stdout)
	var outFile *os.File
	if cmd.Output != "" {
		f, err := os.OpenFile(cmd.Output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer func() {
			if outFile != nil {
				outFile.Close()
			}
		}()
		outFile = f
		out = f
	}

	for _, v := range vars {
		// Skip file-type variables — their values are raw file contents
		// (certificates, PEM keys) that produce invalid .env lines.
		if v.VariableType == "file" {
			if _, err := fmt.Fprintf(out, "# %s (file type, skipped)\n", v.Key); err != nil {
				return fmt.Errorf("write variable %s: %w", v.Key, err)
			}
			continue
		}
		val := v.Value
		// Wrap in double quotes if the value contains special characters.
		// Use double-quote wrapping (not %q / Go escaping) so the output
		// is valid dotenv format readable by shell and glenv's own parser.
		if strings.ContainsAny(val, " \t\n\r\"'\\$") {
			// Escape backslash, newlines, carriage returns, double-quote, and $
			// so the output is safe for shell sourcing.
			val = `"` + strings.NewReplacer(`\`, `\\`, "\r", `\r`, "\n", `\n`, `"`, `\"`, `$`, `\$`).Replace(val) + `"`
		}
		if _, err := fmt.Fprintf(out, "%s=%s\n", v.Key, val); err != nil {
			return fmt.Errorf("write variable %s: %w", v.Key, err)
		}
	}
	if outFile != nil {
		err := outFile.Close()
		outFile = nil // prevent defer from closing again
		if err != nil {
			return fmt.Errorf("close output file: %w", err)
		}
	}
	return nil
}

// DeleteCommand removes one or more remote variables.
type DeleteCommand struct {
	Environment string `short:"e" long:"environment" description:"Environment scope of variable to delete"`
	Force       bool   `long:"force" description:"Skip confirmation prompt"`
	global      *GlobalOptions
}

func (cmd *DeleteCommand) Execute(args []string) error {
	setupColor(cmd.global.NoColor)
	if len(args) == 0 {
		return fmt.Errorf("usage: glenv delete [KEY...]")
	}

	cfg, client, err := buildClientFromGlobal(cmd.global)
	if err != nil {
		return err
	}

	if !cmd.Force {
		fmt.Printf("Delete %d variable(s): %s\n", len(args), strings.Join(args, ", "))
		if !confirm("Confirm deletion?") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	var failed int
	for _, key := range args {
		if err := client.DeleteVariable(appCtx, cfg.GitLab.ProjectID, key, cmd.Environment); err != nil {
			red.Printf("✗ %s: %v\n", key, err)
			failed++
		} else {
			green.Printf("✓ deleted %s\n", key)
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d deletion(s) failed", failed)
	}
	return nil
}

// --- Helpers ---

// resolveWorkers returns the number of workers: CLI flag if set, else config, else default 5.
func resolveWorkers(global *GlobalOptions, cfg *config.Config) int {
	if global.Workers > 0 {
		return global.Workers
	}
	if cfg.RateLimit.MaxConcurrent > 0 {
		return cfg.RateLimit.MaxConcurrent
	}
	return 5
}

// resolveEnvFile returns the .env file path using priority:
// explicit --file flag > environment file from config > default ".env".
func resolveEnvFile(flagFile, environment string, cfg *config.Config) string {
	if flagFile != "" {
		return flagFile
	}
	if environment != "*" {
		if envCfg, ok := cfg.Environments[environment]; ok && envCfg.File != "" {
			return envCfg.File
		}
	}
	return ".env"
}

func buildClientFromGlobal(global *GlobalOptions) (*config.Config, *gitlab.Client, error) {
	cfg, err := config.Load(global.Config)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	// CLI flags override config file values.
	if global.Token != "" {
		cfg.GitLab.Token = global.Token
	}
	if global.Project != "" {
		cfg.GitLab.ProjectID = global.Project
	}
	if global.URL != "" {
		cfg.GitLab.URL = global.URL
	}

	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}

	rps := global.RateLimit
	if rps <= 0 {
		rps = cfg.RateLimit.RequestsPerSecond
	}
	clientCfg := gitlab.ClientConfig{
		BaseURL:             cfg.GitLab.URL,
		Token:               cfg.GitLab.Token,
		RequestsPerSecond:   rps,
		Burst:               max(1, int(rps)),
		RetryMax:            cfg.RateLimit.RetryMax,
		RetryInitialBackoff: cfg.RateLimit.RetryInitialBackoff,
	}

	return cfg, gitlab.NewClient(clientCfg), nil
}

func buildClassifier(cfg *config.Config, noAutoClassify bool) *classifier.Classifier {
	if noAutoClassify {
		return classifier.NewEmpty()
	}
	return classifier.New(classifier.Rules{
		MaskedPatterns: cfg.Classify.MaskedPatterns,
		MaskedExclude:  cfg.Classify.MaskedExclude,
		FilePatterns:   cfg.Classify.FilePatterns,
		FileExclude:    cfg.Classify.FileExclude,
	})
}

func setupColor(noColor bool) {
	if noColor || os.Getenv("NO_COLOR") != "" {
		color.NoColor = true
	}
}

// stdinScanner is a shared scanner for os.Stdin. A single instance is required
// so that buffered data from previous Scan() calls is not lost between calls
// to confirm() (e.g. when --all --delete-missing prompts multiple environments).
var stdinScanner = bufio.NewScanner(os.Stdin)

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	if stdinScanner.Scan() {
		ans := strings.TrimSpace(strings.ToLower(stdinScanner.Text()))
		return ans == "y" || ans == "yes"
	}
	if err := stdinScanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
	} else {
		// EOF on stdin (non-interactive / CI environment): treat as rejection and
		// surface a clear message so the caller knows to use --force.
		fmt.Fprintln(os.Stderr, "stdin is not interactive; pass --force to skip confirmation")
	}
	return false
}

func maskIfNeeded(value, classification string) string {
	if strings.Contains(classification, "masked") {
		return "***"
	}
	return value
}

func printResult(r glsync.Result) {
	if r.Error != nil {
		red.Printf("  ✗ Failed:    %-30s (%v)\n", r.Change.Key, r.Error)
		return
	}
	switch r.Change.Kind {
	case glsync.ChangeCreate:
		val := maskIfNeeded(r.Change.NewValue, r.Change.Classification)
		tags := buildTags(r.Change.Classification)
		green.Printf("  ✓ Created:   %-30s%s\n", r.Change.Key+"="+val, tags)
	case glsync.ChangeUpdate:
		tags := buildTags(r.Change.Classification)
		yellow.Printf("  ↻ Updated:   %-30s%s\n", r.Change.Key, tags)
	case glsync.ChangeDelete:
		red.Printf("  - Deleted:   %s\n", r.Change.Key)
	case glsync.ChangeUnchanged, glsync.ChangeSkipped:
		// Already shown in printDiff; don't repeat during apply.
	}
}

func printDiff(diff glsync.DiffResult) {
	for _, ch := range diff.Changes {
		switch ch.Kind {
		case glsync.ChangeCreate:
			val := maskIfNeeded(ch.NewValue, ch.Classification)
			tags := buildTags(ch.Classification)
			green.Printf("+ %s=%s%s\n", ch.Key, val, tags)
		case glsync.ChangeUpdate:
			yellow.Printf("~ %s: %s → %s\n", ch.Key,
				maskIfNeeded(ch.OldValue, ch.Classification),
				maskIfNeeded(ch.NewValue, ch.Classification))
		case glsync.ChangeDelete:
			red.Printf("- %s\n", ch.Key)
		case glsync.ChangeUnchanged:
			cyan.Printf("= %s\n", ch.Key)
		case glsync.ChangeSkipped:
			gray.Printf("⊘ %s (%s)\n", ch.Key, ch.SkipReason)
		}
	}
}

func printDiffSummary(diff glsync.DiffResult) {
	var created, updated, deleted, unchanged, skipped int
	for _, ch := range diff.Changes {
		switch ch.Kind {
		case glsync.ChangeCreate:
			created++
		case glsync.ChangeUpdate:
			updated++
		case glsync.ChangeDelete:
			deleted++
		case glsync.ChangeUnchanged:
			unchanged++
		case glsync.ChangeSkipped:
			skipped++
		}
	}
	fmt.Printf("\nCreated: %d | Updated: %d | Deleted: %d | Unchanged: %d | Skipped: %d\n",
		created, updated, deleted, unchanged, skipped)
}

const separator = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

func printHeader() {
	fmt.Printf("glenv v%s\n\n", version)
}

func printSyncReport(report glsync.SyncReport) {
	fmt.Println()
	fmt.Println(separator)
	fmt.Printf("  Created: %d | Updated: %d | Deleted: %d | Unchanged: %d | Skipped: %d | Failed: %d\n",
		report.Created, report.Updated, report.Deleted, report.Unchanged, report.Skipped, report.Failed)

	dur := report.Duration.Round(time.Millisecond)
	rate := 0.0
	if report.Duration.Seconds() > 0 {
		rate = float64(report.APICalls) / report.Duration.Seconds()
	}
	fmt.Printf("  Duration: %s | API calls: %d | Rate: %.1f req/s\n", dur, report.APICalls, rate)
	fmt.Println(separator)

	if len(report.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, e := range report.Errors {
			red.Printf("  %v\n", e)
		}
	}
}

func buildTags(classification string) string {
	var tags []string
	if strings.Contains(classification, "masked") {
		tags = append(tags, "[masked]")
	}
	if strings.Contains(classification, "protected") {
		tags = append(tags, "[protected]")
	}
	if len(tags) == 0 {
		return ""
	}
	return " " + strings.Join(tags, " ")
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	appCtx = ctx

	global := &GlobalOptions{}
	parser := flags.NewParser(global, flags.Default)

	// Register subcommands.
	parser.AddCommand("version", "Print version", "Print glenv version", &VersionCommand{})

	syncCmd := &SyncCommand{global: global}
	parser.AddCommand("sync", "Sync .env to GitLab", "Push variables from .env file to GitLab CI/CD", syncCmd)

	diffCmd := &DiffCommand{global: global}
	parser.AddCommand("diff", "Show diff", "Show what would change without applying", diffCmd)

	listCmd := &ListCommand{global: global}
	parser.AddCommand("list", "List variables", "List all GitLab CI/CD variables", listCmd)

	exportCmd := &ExportCommand{global: global}
	parser.AddCommand("export", "Export variables", "Export GitLab CI/CD variables as KEY=VALUE", exportCmd)

	deleteCmd := &DeleteCommand{global: global}
	parser.AddCommand("delete", "Delete variable(s)", "Delete one or more GitLab CI/CD variables", deleteCmd)

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok {
			if flagsErr.Type == flags.ErrHelp {
				os.Exit(0)
			}
		}
		os.Exit(1)
	}
}
