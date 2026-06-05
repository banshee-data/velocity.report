# velocity.report service management aliases
# Installed to /etc/profile.d/ for all interactive shells.

alias velocity-log='journalctl -u velocity-report.service -f --no-hostname -o short | sed "s/^[^[]*\[/[/"'
alias velocity-status='systemctl status velocity-report.service'
alias velocity-bounce='sudo systemctl restart velocity-report.service'
alias velocity-stop='sudo systemctl stop velocity-report.service'
alias velocity-start='sudo systemctl start velocity-report.service'
