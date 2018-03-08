package main // Guide or trip planner

import (
	"net"
	"strings"

	"github.com/shanebarnes/goto/logger"
)

type FinderOp func(*GuideImpl, Role, string) (bool, ShortcutId)

type Plan struct {
	shortcut FastRoute
	finder   FinderOp
}

type GuideImpl struct {
	plans []Plan
}

type Guide interface {
	LoadShortcuts(shortcuts []FastRoute) int
	FindShortcut(role Role, location string) Shortcut
}

func (g *GuideImpl) LoadShortcuts(shortcuts []FastRoute) int {
	n := 0

	// @todo Detect duplicates using maps instead of arrays
	for i := range shortcuts {
		for j := range shortcutsSupported {
			if strings.EqualFold(shortcuts[i].Shortcut, shortcutsSupported[j].Name) {
				switch shortcutsSupported[j].Id {
				case Null:
					n = n + 1
					break
				case AzureBlob:
					g.plans = append(g.plans, Plan{shortcut: shortcuts[i], finder: (*GuideImpl).findShortcutAzureBlob})
					n = n + 1
					break
				default:
					break
				}

				break
			}
		}
	}

	logger.PrintlnInfo("Loaded", n, "shortcuts")

	return n
}

func (g *GuideImpl) findShortcutAzureBlob(role Role, location string) (bool, ShortcutId) {
	found := false

	if role == Client {
		if strings.HasPrefix(location, "AzCopy") ||
			strings.HasPrefix(location, "azcopy") ||
			strings.HasSuffix(location, "azure-storage-go/10.0.2 api-version/2016-05-31 blob") {
			found = true
		}
	}

	return found, AzureBlob
}

func (g *GuideImpl) FindShortcut(route int, role Role, location string, src net.Conn, dst net.Conn) Shortcut {
	var shortcut Shortcut = nil

	for i := range g.plans {
		found, id := g.plans[i].finder(g, role, location)
		if found {
			switch id {
			case AzureBlob:
				shortcut = new(ShortcutAzureBlob)
				shortcut.New(route, src, dst, g.plans[i].shortcut.Exitno, g.plans[i].shortcut.Wormhole)
				logger.PrintlnInfo("{", route, "}", "Found AzCopy shortcut")
				break
			default:
				break
			}

			break
		}
	}

	if shortcut == nil {
		shortcut = new(ShortcutNull)
		shortcut.New(route, src, dst, -1, false)
		logger.PrintlnInfo("{", route, "}", "Shortcut not found")
	}

	return shortcut
}
