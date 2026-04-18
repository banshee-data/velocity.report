# Repository Coverage

[Full report](https://htmlpreview.github.io/?https://github.com/banshee-data/velocity.report/blob/python-coverage-comment-action-data/htmlcov/index.html)

| Name                                       |    Stmts |     Miss |   Cover |   Missing |
|------------------------------------------- | -------: | -------: | ------: | --------: |
| pdf\_generator/\_\_init\_\_.py             |        3 |        0 |    100% |           |
| pdf\_generator/cli/\_\_init\_\_.py         |        0 |        0 |    100% |           |
| pdf\_generator/cli/create\_config.py       |       21 |        0 |    100% |           |
| pdf\_generator/cli/demo.py                 |      122 |        0 |    100% |           |
| pdf\_generator/cli/main.py                 |      550 |       48 |     91% |204-205, 242, 263, 268, 274, 348, 352-353, 395, 401, 714-715, 730-732, 856, 869-870, 873-884, 1027-1031, 1169, 1184, 1204, 1207-1209, 1213-1214, 1232, 1241, 1250, 1262, 1276, 1314, 1374-1377 |
| pdf\_generator/core/\_\_init\_\_.py        |        0 |        0 |    100% |           |
| pdf\_generator/core/api\_client.py         |       47 |        0 |    100% |           |
| pdf\_generator/core/chart\_builder.py      |      381 |       33 |     91% |259-260, 275-276, 407, 425-426, 433-434, 439-440, 481, 502-503, 517, 529-530, 554-561, 611, 632-636, 643-644, 654-655 |
| pdf\_generator/core/chart\_saver.py        |       70 |        3 |     96% |48, 113, 132 |
| pdf\_generator/core/config\_manager.py     |      240 |        1 |     99% |       458 |
| pdf\_generator/core/data\_transformers.py  |       63 |        1 |     98% |        69 |
| pdf\_generator/core/date\_parser.py        |       53 |        0 |    100% |           |
| pdf\_generator/core/dependency\_checker.py |      167 |       10 |     94% |103, 128, 159-160, 204-210, 244, 266-267 |
| pdf\_generator/core/document\_builder.py   |       96 |        5 |     95% |     73-81 |
| pdf\_generator/core/map\_utils.py          |      184 |       11 |     94% |290-307, 316-318 |
| pdf\_generator/core/pdf\_generator.py      |      359 |       23 |     94% |82, 125-128, 225-236, 472-473, 633, 697-698, 720, 769, 789-790 |
| pdf\_generator/core/report\_sections.py    |      153 |        9 |     94% |113-114, 140-142, 150-151, 165-166 |
| pdf\_generator/core/stats\_utils.py        |      133 |       13 |     90% |24-26, 194, 239, 253, 258-259, 284, 300-304 |
| pdf\_generator/core/table\_builders.py     |      220 |        6 |     97% |175, 458, 717, 728-734 |
| pdf\_generator/core/tex\_environment.py    |       45 |        5 |     89% | 69-73, 87 |
| pdf\_generator/core/zip\_utils.py          |      125 |       17 |     86% |67-68, 178-180, 185-187, 254, 281-282, 286-287, 292-293, 298-299 |
|                                  **TOTAL** | **3032** |  **185** | **94%** |           |


## Setup coverage badge

Below are examples of the badges you can use in your main branch `README` file.

### Direct image

[![Coverage badge](https://raw.githubusercontent.com/banshee-data/velocity.report/python-coverage-comment-action-data/badge.svg)](https://htmlpreview.github.io/?https://github.com/banshee-data/velocity.report/blob/python-coverage-comment-action-data/htmlcov/index.html)

This is the one to use if your repository is private or if you don't want to customize anything.

### [Shields.io](https://shields.io) Json Endpoint

[![Coverage badge](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/banshee-data/velocity.report/python-coverage-comment-action-data/endpoint.json)](https://htmlpreview.github.io/?https://github.com/banshee-data/velocity.report/blob/python-coverage-comment-action-data/htmlcov/index.html)

Using this one will allow you to [customize](https://shields.io/endpoint) the look of your badge.
It won't work with private repositories. It won't be refreshed more than once per five minutes.

### [Shields.io](https://shields.io) Dynamic Badge

[![Coverage badge](https://img.shields.io/badge/dynamic/json?color=brightgreen&label=coverage&query=%24.message&url=https%3A%2F%2Fraw.githubusercontent.com%2Fbanshee-data%2Fvelocity.report%2Fpython-coverage-comment-action-data%2Fendpoint.json)](https://htmlpreview.github.io/?https://github.com/banshee-data/velocity.report/blob/python-coverage-comment-action-data/htmlcov/index.html)

This one will always be the same color. It won't work for private repos. I'm not even sure why we included it.

## What is that?

This branch is part of the
[python-coverage-comment-action](https://github.com/marketplace/actions/python-coverage-comment)
GitHub Action. All the files in this branch are automatically generated and may be
overwritten at any moment.