# Agents

velocity.report uses seven specialised agent personas. Each brings a distinct discipline and voice to the project.

|                                 |                                                                                                              |
| :-----------------------------: | :----------------------------------------------------------------------------------------------------------- |
|  ![Euler](portraits/euler.jpg)  | [**Euler**](euler.agent.md) - Patient mathematician. Every number has provenance, every claim has bounds.    |
|  ![Grace](portraits/grace.jpg)  | [**Grace**](grace.agent.md) - Pirate architect. Makes the complex approachable and the abstract tangible.    |
| ![Appius](portraits/appius.jpg) | [**Appius**](appius.agent.md) - Long-sighted developer. Builds infrastructure that outlasts its authors.     |
| ![malory](portraits/malory.jpg) | [**malory**](malory.agent.md) ~ red-team security researcher. finds what's broken before someone else does   |
|    ![Flo](portraits/flo.jpg)    | [**Flo**](flo.agent.md) - Evidence-based PM. Creates the conditions where good work happens naturally.       |
|  ![Terry](portraits/terry.jpg)  | [**Terry**](terry.agent.md) - House writer. Humane satire, lucid prose, zero tolerance for pompous nonsense. |
|   ![Ruth](portraits/ruth.jpg)   | [**Ruth**](ruth.agent.md) - Executive judge. Measured scope, principled restraint, durable decisions.        |

## Attribution

## Platform Parity

Agent personas are paired across both platforms: `.github/agents/` (Copilot) and `.claude/agents/` (Claude Code). Run `make check-agent-drift` to verify alignment.

**Workflow skills** live in `.claude/skills/` (13 skills) and are invoked as slash commands in Claude Code. Copilot prompts (`.github/prompts/`, 2 prompts) cover a smaller set because Copilot prompts cannot run terminal commands or orchestrate multi-step workflows. This asymmetry is intentional: skills that require tool access (`ship-change`, `release-prep`, `security-review`, etc.) have no Copilot equivalent. Prompts that are purely advisory (`svelte-ux-review`) may exist only in Copilot.

## Attribution

- **Leonhard Euler** — portrait by Jakob Emanuel Handmann, 1753. Kunstmuseum Basel. Public domain. [Wikimedia Commons](https://commons.wikimedia.org/wiki/File:Leonhard_Euler.jpg).
- **Grace Hopper** — official US Navy photograph, 1984. Public domain (US government work). [Wikimedia Commons](<https://commons.wikimedia.org/wiki/File:Commodore_Grace_M._Hopper,_USN_(covered).jpg>).
- **Appius Claudius Caecus** — Roman bust, Vatican Museums, Braccio Chiaramonti. Public domain. [Wikimedia Commons](https://commons.wikimedia.org/wiki/File:Musei_vaticani,_braccio_chiaramonti,_busto_02.JPG).
- **malory** — photograph by [parb](https://www.flickr.com/photos/parb/), 2014. [CC BY-NC-ND 2.0](https://creativecommons.org/licenses/by-nc-nd/2.0/). [Flickr](https://www.flickr.com/photos/parb/14569194548/).
- **Florence Nightingale** — photograph by Henry Hering, c. 1858. National Portrait Gallery, London (NPG x82368). Public domain. [Wikimedia Commons](<https://commons.wikimedia.org/wiki/File:Florence_Nightingale_(H_Hering_NPG_x82368).jpg>).
- **Terry Pratchett** — photograph by Luigi Novi, 2012. [CC BY 3.0](https://creativecommons.org/licenses/by/3.0/). [Wikimedia Commons](https://commons.wikimedia.org/wiki/File:10.12.12TerryPratchettByLuigiNovi1.jpg).
- **Ruth Bader Ginsburg** — official Supreme Court portrait, 2016. Collection of the Supreme Court of the United States. Public domain (US government work). [Wikimedia Commons](https://commons.wikimedia.org/wiki/File:Ruth_Bader_Ginsburg_2016_portrait.jpg).
