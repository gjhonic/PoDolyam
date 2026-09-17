.DEFAULT_GOAL := help

SHELL := cmd.exe
.SHELLFLAGS := /D /S /C

.PHONY: help test build

help:
	@chcp 65001 >nul && node -e "process.stdout.write('\n\x1b[1m\u{1F3A8}  PoDolyam \u2014 Windows\x1b[0m\n\n  \u{1F9EA}  \x1b[36mmake test\x1b[0m   \u041f\u0440\u043e\u0432\u0435\u0440\u0438\u0442\u044c Go, Vue \u0438 \u0434\u0435\u043d\u0435\u0436\u043d\u043e\u0435 \u044f\u0434\u0440\u043e\n  \u{1F4E6}  \x1b[35mmake build\x1b[0m  \u0421\u043e\u0431\u0440\u0430\u0442\u044c build\\PoDolyam.exe\n  \u{1F4A1}  \x1b[33mmake help\x1b[0m   \u041f\u043e\u043a\u0430\u0437\u0430\u0442\u044c \u044d\u0442\u0443 \u043f\u043e\u0434\u0441\u043a\u0430\u0437\u043a\u0443\n\n')"

test:
	@scripts\windows.cmd install
	@scripts\windows.cmd test

build:
	@scripts\windows.cmd install
	@scripts\windows.cmd build
