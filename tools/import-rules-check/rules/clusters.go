// Copyright 2026 opendbx contributors. See LICENSE.
//
// Cluster restrictions: bans intra-layer cross-cluster imports that the
// layer matrix can't express on its own (spec-0.2 § 2.2 重要细则 #4-5).
//
// Author: sqlrush
package rules

import (
	"fmt"
	"strings"
)

// ClusterRule denies an import edge when both endpoints sit under the
// configured prefixes AND (for self-reflexive rules like services-mutual)
// the leaf segments differ.
type ClusterRule struct {
	FromPrefix string
	ToPrefix   string
	Reason     string
	// SelfReflexive indicates the rule applies between distinct *children*
	// of the same prefix (e.g., services/mcp ↛ services/auth, but
	// services/mcp/internal → services/mcp/utility is fine).
	SelfReflexive bool
}

// Clusters lists every spec-0.2 § 2.2 cluster restriction.
var Clusters = []ClusterRule{
	{
		FromPrefix:    ModulePrefix + "internal/app/services/",
		ToPrefix:      ModulePrefix + "internal/app/services/",
		Reason:        "services must communicate via bootstrap-assembled interfaces, not direct imports",
		SelfReflexive: true,
	},
	{
		FromPrefix:    ModulePrefix + "internal/domain/db/",
		ToPrefix:      ModulePrefix + "internal/domain/db/",
		Reason:        "DB drivers are isolated; cross-driver communication is illegal — use the domain/db.Driver interface",
		SelfReflexive: true,
	},
	{
		FromPrefix:    ModulePrefix + "internal/app/cli/render/scrollback",
		ToPrefix:      ModulePrefix + "internal/app/cli/components",
		Reason:        "scrollback is a render subsystem; components are higher-level (no upward import allowed)",
		SelfReflexive: false,
	},
}

// CheckCluster returns "" if the edge passes all cluster rules, or a
// violation reason for the first rule it trips.
func CheckCluster(from, to string) string {
	for _, r := range Clusters {
		if !strings.HasPrefix(from, r.FromPrefix) {
			continue
		}
		if !strings.HasPrefix(to, r.ToPrefix) {
			continue
		}
		if r.SelfReflexive {
			fromLeaf := firstSegment(strings.TrimPrefix(from, r.FromPrefix))
			toLeaf := firstSegment(strings.TrimPrefix(to, r.ToPrefix))
			if fromLeaf == "" || toLeaf == "" || fromLeaf == toLeaf {
				continue
			}
			return fmt.Sprintf("cluster: %s/%s ↛ %s/%s — %s",
				strings.TrimSuffix(r.FromPrefix, "/"), fromLeaf,
				strings.TrimSuffix(r.ToPrefix, "/"), toLeaf,
				r.Reason)
		}
		return fmt.Sprintf("cluster: %s ↛ %s — %s", from, to, r.Reason)
	}
	return ""
}

// firstSegment returns the first slash-delimited segment of a relative path.
// Examples:
//
//	"mcp"          -> "mcp"
//	"mcp/server"   -> "mcp"
//	""             -> ""
func firstSegment(rel string) string {
	if idx := strings.IndexByte(rel, '/'); idx >= 0 {
		return rel[:idx]
	}
	return rel
}
