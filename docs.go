// Package inkwire carries the documents that describe its own schema.
//
// They live here rather than beside the command because go:embed cannot reach
// up out of its own directory, and the files have to stay at the repository
// root for anyone reading the project on the web. Embedding the same files
// rather than a copy of them is the point: a second copy would be one more
// thing that can quietly stop matching the first.
package inkwire

import _ "embed"

// Schema is the Scene Schema reference, printed by `inkwire schema`. A binary
// that carries it can tell whatever drives it what a scene document may
// contain, with no network and nothing else to download.
//
//go:embed SCHEMA.md
var Schema string

// SchemaChinese is the same document in Simplified Chinese.
//
//go:embed SCHEMA.zh-CN.md
var SchemaChinese string
