package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
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

// GlobalOptions holds flags shared across all commands.
type GlobalOptions struct {
	Config    string  `short:"c" long:"config" description:"Path to .glenv.yml config file"`
	Token     string  `long:"token" env:"GITLAB_TOKEN" description:"GitLab private token"`
	Project   string  `long:"project" env:"GITLAB_PROJECT_ID" description:"GitLab project ID"`
	URL       string  `long:"url" env:"GITLAB_URL" description:"GitLab base URL"`
	DryRun    bool    `long:"dry-run" description:"Print planned changes without applying them"`
	Debug     bool    `long:"debug" description:"Enable debug output"`
	NoColor   bool    `long:"no-color" description:"Disable colored output"`
	Workers   int     `long:"workers" description:"Number of concurrent workers" default:"5"`
	RateLimit float64 `long:"rate-limit" description:"Max API requests per second" default:"10"`
}

// VersionCommand prints the build version.
type VersionCommand struct{}

func (cmd *VersionCommand) Execute(args []string) error {
	fmt.Printf("glenv version %s\n", version)
	return nil
}

// SyncCommand pushes local .env variables to GitLab.
type SyncCommand struct {
	File          string `short:"f" long:"file" description:"Path to .env file" default:".env"`
	Environment   string `short:"e" long:"environment" description:"GitLab environment scope" default:"*"`
	All           bool   `short:"a" long:"all" description:"Sync all environments defined in config"`
	DeleteMissing bool   `long:"delete-missing" description:"Delete remote variables not present in .env file"`
	NoAutoClassify bool  `long:"no-auto-classify" description:"Disable automatic variable classification"`
	Force         bool   `long:"force" description:"Skip confirmation prompt"`
	global        *GlobalOptions
}

func (cmd *SyncCommand) Execute(args []string) error {
	setupColor(cmd.global.NoColor)
	cfg, client, err := buildClientFromGlobal(cmd.global)
	if err != nil {
		return err
	}

	parsed, err := envfile.ParseFile(cmd.File)
	if err != nil {
		return fmt.Errorf("parse %s: %w", cmd.File, err)
	}

	cl := buildClassifier(cfg, cmd.NoAutoClassify)
	opts := glsync.Options{
		Workers:       cmd.global.Workers,
		DryRun:        cmd.global.DryRun,
		DeleteMissing: cmd.DeleteMissing,
		Environment:   cmd.Environment,
	}
	engine := glsync.NewEngine(client, cl, opts, cfg.GitLab.ProjectID)

	remote, err := client.ListVariables(appCtx, cfg.GitLab.ProjectID, gitlab.ListOptions{EnvironmentScope: cmd.Environment})
	if err != nil {
		return fmt.Errorf("list remote variables: %w", err)
	}

	diff := engine.Diff(appCtx, parsed.Variables, remote, cmd.Environment)

	if !cmd.Force && !cmd.global.DryRun {
		printDiff(diff)
		if !confirm("Apply these changes?") {
			fmt.Println("Aborted.")
			return nil
		}
	} else if cmd.global.DryRun {
		printDiff(diff)
		return nil
	}

	report := engine.ApplyWithCallback(appCtx, diff, func(r glsync.Result) {
		printResult(r)
	})

	printSyncReport(report)
	return nil
}

// DiffCommand shows what would change without applying.
type DiffCommand struct {
	File        string `short:"f" long:"file" description:"Path to .env file" default:".env"`
	Environment string `short:"e" long:"environment" description:"GitLab environment scope" default:"*"`
	global      *GlobalOptions
}

func (cmd *DiffCommand) Execute(args []string) error {
	setupColor(cmd.global.NoColor)
	cfg, client, err := buildClientFromGlobal(cmd.global)
	if err != nil {
		return err
	}

	parsed, err := envfile.ParseFile(cmd.File)
	if err != nil {
		return fmt.Errorf("parse %s: %w", cmd.File, err)
	}

	cl := buildClassifier(cfg, false)
	opts := glsync.Options{
		Workers:     cmd.global.Workers,
		Environment: cmd.Environment,
	}
	engine := glsync.NewEngine(client, cl, opts, cfg.GitLab.ProjectID)

	remote, err := client.ListVariables(appCtx, cfg.GitLab.ProjectID, gitlab.ListOptions{EnvironmentScope: cmd.Environment})
	if err != nil {
		return fmt.Errorf("list remote variables: %w", err)
	}

	diff := engine.Diff(appCtx, parsed.Variables, remote, cmd.Environment)
	printDiff(diff)
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
	if cmd.Output != "" {
		f, err := os.Create(cmd.Output)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		out = f
	}

	for _, v := range vars {
		val := v.Value
		if strings.ContainsAny(val, " \t\n\"'") {
			val = fmt.Sprintf("%q", val)
		}
		if _, err := fmt.Fprintf(out, "%s=%s\n", v.Key, val); err != nil {
			return fmt.Errorf("write variable %s: %w", v.Key, err)
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
			color.Red("✗ %s: %v\n", key, err)
			failed++
		} else {
			color.Green("✓ deleted %s\n", key)
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d deletion(s) failed", failed)
	}
	return nil
}

// --- Helpers ---

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

	clientCfg := gitlab.ClientConfig{
		BaseURL:             cfg.GitLab.URL,
		Token:               cfg.GitLab.Token,
		RequestsPerSecond:   global.RateLimit,
		Burst:               int(global.RateLimit),
		RetryMax:            cfg.RateLimit.RetryMax,
		RetryInitialBackoff: cfg.RateLimit.RetryInitialBackoff,
	}
	if clientCfg.RequestsPerSecond <= 0 {
		clientCfg.RequestsPerSecond = cfg.RateLimit.RequestsPerSecond
	}

	return cfg, gitlab.NewClient(clientCfg), nil
}

func buildClassifier(cfg *config.Config, noAutoClassify bool) *classifier.Classifier {
	if noAutoClassify {
		return classifier.New(classifier.Rules{})
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

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		ans := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return ans == "y" || ans == "yes"
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
		color.Red("  ✗ %s: %v\n", r.Change.Key, r.Error)
		return
	}
	switch r.Change.Kind {
	case glsync.ChangeCreate:
		val := maskIfNeeded(r.Change.NewValue, r.Change.Classification)
		tags := buildTags(r.Change.Classification)
		color.Green("  ✓ %s=%s%s\n", r.Change.Key, val, tags)
	case glsync.ChangeUpdate:
		val := maskIfNeeded(r.Change.NewValue, r.Change.Classification)
		tags := buildTags(r.Change.Classification)
		color.Yellow("  ↻ %s=%s%s\n", r.Change.Key, val, tags)
	case glsync.ChangeDelete:
		color.Red("  - %s\n", r.Change.Key)
	case glsync.ChangeUnchanged:
		fmt.Printf("  = %s\n", r.Change.Key)
	case glsync.ChangeSkipped:
		color.HiBlack("  ⊘ %s (%s)\n", r.Change.Key, r.Change.SkipReason)
	}
}

func printDiff(diff glsync.DiffResult) {
	for _, ch := range diff.Changes {
		switch ch.Kind {
		case glsync.ChangeCreate:
			val := maskIfNeeded(ch.NewValue, ch.Classification)
			tags := buildTags(ch.Classification)
			color.Green("+ %s=%s%s\n", ch.Key, val, tags)
		case glsync.ChangeUpdate:
			color.Yellow("~ %s: %s → %s\n", ch.Key,
				maskIfNeeded(ch.OldValue, ch.Classification),
				maskIfNeeded(ch.NewValue, ch.Classification))
		case glsync.ChangeDelete:
			color.Red("- %s\n", ch.Key)
		case glsync.ChangeUnchanged:
			fmt.Printf("= %s\n", ch.Key)
		case glsync.ChangeSkipped:
			color.HiBlack("⊘ %s (%s)\n", ch.Key, ch.SkipReason)
		}
	}
}

func printSyncReport(report glsync.SyncReport) {
	fmt.Println()
	fmt.Printf("Created: %d | Updated: %d | Unchanged: %d | Skipped: %d | Failed: %d\n",
		report.Created, report.Updated, report.Unchanged, report.Skipped, report.Failed)

	dur := report.Duration.Round(time.Millisecond)
	rate := 0.0
	if report.Duration.Seconds() > 0 {
		rate = float64(report.APICalls) / report.Duration.Seconds()
	}
	fmt.Printf("Duration: %s | API calls: %d | Rate: %.1f req/s\n", dur, report.APICalls, rate)

	if len(report.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, e := range report.Errors {
			color.Red("  %v\n", e)
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
