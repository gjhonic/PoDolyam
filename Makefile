.DEFAULT_GOAL := help

SHELL := cmd.exe
.SHELLFLAGS := /D /S /C

.PHONY: help test build

help:
	@node -e "console.log('\n\x1b[1m🎨  PoDolyam — Windows\x1b[0m\n\n  🧪  \x1b[36mmake test\x1b[0m   Проверить Go, Vue и денежное ядро\n  📦  \x1b[35mmake build\x1b[0m  Собрать build\\PoDolyam.exe\n  💡  \x1b[33mmake help\x1b[0m   Показать эту подсказку\n')"

test:
	@scripts\windows.cmd install
	@scripts\windows.cmd test

build:
	@scripts\windows.cmd install
	@scripts\windows.cmd build
