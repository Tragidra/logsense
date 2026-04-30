// Package cluster groups log events into templates using the Drain algorithm.
//
// The Tree maintains a fixed-depth parse tree, each event walks the tree to
// find a matching leaf based on event length and leading tokens. At each leaf,
// the event is compared against existing templates by token-level similarity;
// a new template is created when no existing one exceeds the configured
// threshold.

package cluster
