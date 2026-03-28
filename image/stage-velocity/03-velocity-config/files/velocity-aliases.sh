# velocity.report service management aliases
# Installed to /etc/profile.d/ for all interactive shells.

alias velocity-log='journalctl -u velocity-report.service -f'
alias velocity-status='systemctl status velocity-report.service'
alias velocity-restart='sudo systemctl restart velocity-report.service'
alias velocity-stop='sudo systemctl stop velocity-report.service'
alias velocity-start='sudo systemctl start velocity-report.service'
