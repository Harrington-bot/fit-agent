package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jogvan-k/fit-agent/internal/syncorch"
)

func newSyncWorkoutsCmd() *cobra.Command {
	var (
		rf    rangeFlags
		prune bool
	)
	cmd := &cobra.Command{
		Use:   "sync-workouts",
		Short: "Two-way sync of planned workouts with intervals.icu",
		Long: `sync-workouts is the agent's primary workout-calendar command.

It refreshes the remote event inventory, then runs two phases:

  1. Push: every agent-authored fit-agent/planned-workouts/*.md file in
     range is diffed against the cached intervals.icu snapshot and the
     resulting create / update / delete actions are sent to icu (same
     behaviour as the legacy ` + "`push-workouts`" + ` command).

  2. Pull: every WORKOUT-category event in range is fetched fresh from
     icu and written to ` + "`.cache/events/<id>.json`" + `. The pull
     step then rewrites the machine-owned YAML block (delimited by the
     ` + "`<!-- fit-agent:icu:begin -->`" + ` / ` + "`<!-- fit-agent:icu:end -->`" + `
     sentinels) inside each ` + "`fit-agent/planned-workouts/YYYY-MM-DD.md`" + `,
     creating the file with a default skeleton when none exists. The
     agent's frontmatter, prose, and ` + "`fit-workout`" + ` fences are
     preserved byte-for-byte.

The preflight refresh prevents duplicate events after a deleted local
cache: matching remote events are recovered by date and name before a
create can be planned. The final pull returns newly-authored workouts
and stamps their server-assigned ids in locally-authored files.

Pass --prune to also DELETE icu events that no longer have a matching
locally-authored markdown file (otherwise such events are reported as
'skip' during the push step).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, dryRun, err := resolveRuntime(cmd)
			if err != nil {
				return err
			}
			r, err := rf.parse(res.Location)
			if err != nil {
				return err
			}
			sctx := syncorch.Context{
				Client:    res.Client,
				AthleteID: res.Profile.IcuAthleteID,
				Layout:    res.Layout,
				Location:  res.Location,
				DryRun:    dryRun,
				Prune:     prune,
				Logger:    makeLogger(cmd),
			}
			result, err := syncorch.Sync(ctxOrBackground(cmd), sctx, r)
			out := stdoutOrStderrForResults(cmd)
			fmt.Fprintf(out, "sync-workouts: %s\n", result)
			return err
		},
	}
	rf.bind(cmd)
	cmd.Flags().BoolVar(&prune, "prune", false, "DELETE icu events that no longer appear in markdown")
	return cmd
}
