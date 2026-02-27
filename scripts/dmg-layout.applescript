-- dmg-layout.applescript -- Configure Finder icon layout for a drag-to-install DMG.
--
-- Usage:
--   osascript scripts/dmg-layout.applescript <volume-name> <app-name> [extra ...]
--
-- Positions the app icon at the left (x=100, y=70) and an Applications
-- alias at the right (x=300, y=70).  Any extra items are evenly spaced
-- on a second row (y=230).
--
-- Window: 520 × 400 (bounds {100, 100, 620, 500}), icon view, 72 px icons,
-- no toolbar or sidebar.

on run argv
	set volumeName to item 1 of argv
	set appName to item 2 of argv

	tell application "Finder"
		-- Wait for Finder to discover the newly mounted volume (up to 15 s).
		set maxAttempts to 15
		set diskFound to false
		repeat with attempt from 1 to maxAttempts
			try
				set volRef to disk volumeName
				set diskFound to true
				exit repeat
			on error
				delay 1
			end try
		end repeat
		if not diskFound then
			error "Finder never discovered disk '" & volumeName & "' after " & maxAttempts & " seconds."
		end if

		tell disk volumeName
			open
			set current view of container window to icon view
			set toolbar visible of container window to false
			set statusbar visible of container window to false
			set bounds of container window to {100, 100, 620, 500}
			set theViewOptions to icon view options of container window
			set arrangement of theViewOptions to not arranged
			set icon size of theViewOptions to 72

			-- Row 1: app on the left, Applications on the right.
			set position of item appName of container window to {100, 70}
			set position of item "Applications" of container window to {300, 70}

			-- Row 2: extras evenly spaced below.
			set nExtras to (count of argv) - 2
			if nExtras > 0 then
				set winWidth to 520
				set gap to winWidth div (nExtras + 1)
				repeat with i from 3 to count of argv
					set extraName to item i of argv
					set extraIndex to i - 2
					set xPos to gap * extraIndex
					set position of item extraName of container window to {xPos, 230}
				end repeat
			end if

			close
			open
			update without registering applications
			delay 2
			close
		end tell
	end tell
end run
