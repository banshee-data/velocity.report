-- dmg-layout.applescript -- Configure Finder icon layout for a drag-to-install DMG.
--
-- Usage:
--   osascript scripts/dmg-layout.applescript <volume-name> <app-name> [name:x ...]
--
-- Positions the app icon at the left (x=130) and an Applications alias at
-- the right (x=390).  Any extra items are passed as "name:x" pairs and
-- placed at the given x coordinate.  All items sit at y=160.
--
-- Window: 520 × 340, icon view, 72 px icons, no toolbar or sidebar.

on run argv
	set volumeName to item 1 of argv
	set appName to item 2 of argv

	tell application "Finder"
		tell disk volumeName
			open
			set current view of container window to icon view
			set toolbar visible of container window to false
			set statusbar visible of container window to false
			set bounds of container window to {100, 100, 620, 440}
			set theViewOptions to icon view options of container window
			set arrangement of theViewOptions to not arranged
			set icon size of theViewOptions to 72
			set position of item appName of container window to {130, 160}

			-- Position any extra items passed as "name:x" pairs.
			repeat with i from 3 to count of argv
				set pair to item i of argv
				set AppleScript's text item delimiters to ":"
				set parts to text items of pair
				set extraName to item 1 of parts
				set extraX to (item 2 of parts) as integer
				set AppleScript's text item delimiters to ""
				set position of item extraName of container window to {extraX, 160}
			end repeat

			set position of item "Applications" of container window to {390, 160}
			close
			open
			update without registering applications
			delay 2
			close
		end tell
	end tell
end run
