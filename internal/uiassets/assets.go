package uiassets

import "embed"

//go:embed ui.css
var CSS string

// LogoSVG is the original offline PairShell application mark.
//
//go:embed pairshell-logo.svg
var LogoSVG string

//go:embed invite.html
var Invite string

//go:embed control.html
var Control string

//go:embed merchant.html
var Merchant string

//go:embed merchant.js
var MerchantJS string

//go:embed desktop.js
var DesktopJS string

//go:embed history.js
var HistoryJS string

// Terminal assets are pinned, licensed npm distributions shipped offline.
//
//go:embed terminal-assets/*
var TerminalAssets embed.FS
