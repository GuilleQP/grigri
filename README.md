# grigri

[![License](https://img.shields.io/github/license/GuilleQP/grigri)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/GuilleQP/grigri)](go.mod)
[![kagent](https://img.shields.io/badge/kagent-1.x%20(v1alpha3)-blue)](https://github.com/kagent-dev/kagent)


Long agent tasks drift, forget their instructions, and take risky steps without a second look. grigri is a lightweight Go **harness** for [kagent](https://github.com/kagent-dev/kagent) that adds a safety layer around the agents it runs: anchored context that is never summarized away, bounded pitches of work, and a belayer agent that reviews risky steps and catches drift.

Named after the climbing belay device. Not affiliated with or endorsed by Petzl.

## License

Apache 2.0. `cmd/kagent-harness` is adapted from kagent (Apache 2.0).
