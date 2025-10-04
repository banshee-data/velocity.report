import sys, os

# Ensure imports from internal/report/query_data resolve when running this file directly
sys.path.insert(0, os.path.join(os.getcwd(), "internal/report/query_data"))
from get_stats import _plot_stats_page
import datetime

# create synthetic stats with three segments separated by rows with null speeds
stats = []
base = datetime.datetime(2025, 6, 1, 0, 0)
# segment 1 (3 points)
for i in range(3):
    stats.append(
        {
            "StartTime": (base + datetime.timedelta(hours=i)).isoformat(),
            "P50Speed": 10 + i,
            "P85Speed": 12 + i,
            "P98Speed": 13 + i,
            "MaxSpeed": 15 + i,
            "Count": 100,
        }
    )
# gap rows with null speeds (but non-zero counts)
for i in range(3, 5):
    stats.append(
        {
            "StartTime": (base + datetime.timedelta(hours=i)).isoformat(),
            "P50Speed": None,
            "P85Speed": None,
            "P98Speed": None,
            "MaxSpeed": None,
            "Count": 0,
        }
    )
# segment 2 (3 points)
for i in range(5, 8):
    stats.append(
        {
            "StartTime": (base + datetime.timedelta(hours=i)).isoformat(),
            "P50Speed": 20 + i,
            "P85Speed": 22 + i,
            "P98Speed": 23 + i,
            "MaxSpeed": 25 + i,
            "Count": 80,
        }
    )
# gap rows with null speeds
for i in range(8, 10):
    stats.append(
        {
            "StartTime": (base + datetime.timedelta(hours=i)).isoformat(),
            "P50Speed": None,
            "P85Speed": None,
            "P98Speed": None,
            "MaxSpeed": None,
            "Count": 0,
        }
    )
# segment 3 (3 points)
for i in range(10, 13):
    stats.append(
        {
            "StartTime": (base + datetime.timedelta(hours=i)).isoformat(),
            "P50Speed": 30 + i,
            "P85Speed": 32 + i,
            "P98Speed": 33 + i,
            "MaxSpeed": 35 + i,
            "Count": 60,
        }
    )

fig = _plot_stats_page(stats, "test", "mph")
# inspect line artists to confirm segments
lines = fig.axes[0].get_lines()
print("number of line artists on primary axis:", len(lines))
for li in lines:
    xdata = li.get_xdata()
    ydata = li.get_ydata()
    print("len segment", len(xdata), "label", li.get_label())

# Save the generated stats PDF to inspect visually
out = "out-test_stats.pdf"
fig.savefig(out, bbox_inches="tight")
print("wrote", out)
