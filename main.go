package main

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/tobischo/gokeepasslib/v3"
	"golang.org/x/term"
)

type Entry struct {
	Title    string
	Username string
	Password string
	URL      string
	Notes    string
	Group    string
}

func getPassword() (string, error) {
	fmt.Print("Enter database password: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(bytePassword), nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: kdbx-viewer <database.kdbx>")
		os.Exit(1)
	}

	password, err := getPassword()
	if err != nil {
		log.Fatal("Error reading password:", err)
	}

	app := tview.NewApplication()

	// Create the layout with a title
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Add a title bar
	titleBar := tview.NewTextView().
		SetText("KeePass Database Viewer (ESC to exit)").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorYellow)

	// Create main content area
	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	list := tview.NewList().
		ShowSecondaryText(true).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorBlack).
		SetSelectedBackgroundColor(tcell.ColorGreen)

	details := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true).
		SetTextColor(tcell.ColorWhite).
		SetBorder(true).
		SetTitle("Entry Details")

	// Load and decode database
	file, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	db := gokeepasslib.NewDatabase()
	db.Credentials = gokeepasslib.NewPasswordCredentials(password)

	if err := gokeepasslib.NewDecoder(file).Decode(db); err != nil {
		log.Fatal("Failed to decode database: ", err)
	}

	db.UnlockProtectedEntries()

	// Extract entries with group information
	var entries []Entry
	for _, group := range db.Content.Root.Groups {
		entries = append(entries, extractEntriesWithGroup(group, group.Name)...)
	}

	// Populate list with meaningful information
	for i, entry := range entries {
		secondaryText := ""
		if entry.Username != "" {
			secondaryText = fmt.Sprintf("[%s] %s", entry.Group, entry.Username)
		} else {
			secondaryText = fmt.Sprintf("[%s]", entry.Group)
		}
		list.AddItem(entry.Title, secondaryText, rune('a'+i), nil)
	}

	// Handle list selection
	list.SetSelectedFunc(func(index int, _ string, _ string, _ rune) {
		entry := entries[index]
		details.Clear()
		fmt.Fprintf(details, "[yellow]Group:[white] %s\n\n", entry.Group)
		fmt.Fprintf(details, "[yellow]Title:[white] %s\n", entry.Title)
		fmt.Fprintf(details, "[yellow]Username:[white] %s\n", entry.Username)
		fmt.Fprintf(details, "[yellow]Password:[white] %s\n", entry.Password)
		if entry.URL != "" {
			fmt.Fprintf(details, "[yellow]URL:[white] %s\n", entry.URL)
		}
		if entry.Notes != "" {
			fmt.Fprintf(details, "\n[yellow]Notes:[white]\n%s\n", entry.Notes)
		}
	})

	// Layout setup
	flex.AddItem(titleBar, 1, 1, false).
		AddItem(tview.NewFlex().
			AddItem(list, 0, 1, true).
			AddItem(details, 0, 2, false),
			0, 8, true)

	// Input handling
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			app.Stop()
		case tcell.KeyTab:
			if list.HasFocus() {
				app.SetFocus(details)
			} else {
				app.SetFocus(list)
			}
		}
		return event
	})

	if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		log.Fatal(err)
	}
}

func extractEntriesWithGroup(group gokeepasslib.Group, groupPath string) []Entry {
	var entries []Entry

	for _, entry := range group.Entries {
		// Skip empty entries
		if entry.GetTitle() == "" {
			continue
		}

		e := Entry{
			Title:    entry.GetTitle(),
			Username: entry.GetContent("UserName"),
			Password: entry.GetPassword(),
			URL:      entry.GetContent("URL"),
			Notes:    entry.GetContent("Notes"),
			Group:    groupPath,
		}
		entries = append(entries, e)
	}

	// Recursively process subgroups
	for _, subgroup := range group.Groups {
		newGroupPath := groupPath
		if subgroup.Name != "" {
			if newGroupPath != "" {
				newGroupPath += " / "
			}
			newGroupPath += subgroup.Name
		}
		entries = append(entries, extractEntriesWithGroup(subgroup, newGroupPath)...)
	}

	return entries
}
