# Repository Coverage

[Full report](https://htmlpreview.github.io/?https://github.com/banshee-data/velocity.report/blob/python-coverage-comment-action-data/htmlcov/index.html)

| Name                                       |    Stmts |     Miss |   Cover |   Missing |
|------------------------------------------- | -------: | -------: | ------: | --------: |
| pdf\_generator/\_\_init\_\_.py             |        3 |        0 |    100% |           |
| pdf\_generator/cli/\_\_init\_\_.py         |        0 |        0 |    100% |           |
| pdf\_generator/cli/create\_config.py       |       21 |        0 |    100% |           |
| pdf\_generator/cli/demo.py                 |      122 |        0 |    100% |           |
| pdf\_generator/cli/main.py                 |      542 |       47 |     91% |204-205, 242, 263, 268, 274, 348, 352-353, 395, 401, 714-715, 730-732, 856, 869-870, 873-884, 1027-1031, 1166, 1181, 1201, 1204-1206, 1210-1211, 1229, 1238, 1247, 1259, 1273, 1311, 1371 |
| pdf\_generator/core/\_\_init\_\_.py        |        0 |        0 |    100% |           |
| pdf\_generator/core/api\_client.py         |       47 |        0 |    100% |           |
| pdf\_generator/core/chart\_builder.py      |      381 |       33 |     91% |259-260, 275-276, 415, 433-434, 441-442, 447-448, 489, 510-511, 525, 537-538, 562-569, 619, 640-644, 651-652, 662-663 |
| pdf\_generator/core/chart\_saver.py        |       70 |        3 |     96% |48, 113, 132 |
| pdf\_generator/core/config\_manager.py     |      261 |        2 |     99% |  458, 567 |
| pdf\_generator/core/data\_transformers.py  |       63 |        1 |     98% |        69 |
| pdf\_generator/core/date\_parser.py        |       53 |        0 |    100% |           |
| pdf\_generator/core/dependency\_checker.py |      167 |       10 |     94% |103, 128, 159-160, 204-210, 244, 266-267 |
| pdf\_generator/core/document\_builder.py   |       97 |        5 |     95% |   107-115 |
| pdf\_generator/core/map\_utils.py          |      183 |       10 |     95% |288-299, 308-310 |
| pdf\_generator/core/pdf\_generator.py      |      332 |       19 |     94% |128, 226-237, 473-474, 634, 685-686, 710, 773-774 |
| pdf\_generator/core/report\_sections.py    |      159 |       13 |     92% |54, 128-129, 155-157, 165-166, 180-181, 253, 296, 421 |
| pdf\_generator/core/stats\_utils.py        |      133 |       13 |     90% |24-26, 194, 239, 253, 258-259, 284, 300-304 |
| pdf\_generator/core/table\_builders.py     |      222 |        9 |     96% |193, 332, 406, 468, 668, 733, 744-750 |
| pdf\_generator/core/tex\_environment.py    |       40 |        5 |     88% | 55-59, 73 |
| pdf\_generator/core/zip\_utils.py          |      125 |       17 |     86% |67-68, 178-180, 185-187, 254, 281-282, 286-287, 292-293, 298-299 |
| **TOTAL**                                  | **3021** |  **187** | **94%** |           |


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