package migrations

import "embed"

// SQL входит в бинарник: мигратор не зависит от рабочего каталога процесса.
//
//go:embed *.sql
var Files embed.FS
