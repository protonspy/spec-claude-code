# A starting point, not a base image — this file is yours from the moment it is
# written. scc scaffolds it once and never updates it, the way it seeds docs/.
#
# `/scc-init` adjusts it to this project: the language toolchain, the package
# manager, and whatever the delivery gate's commands actually need. A container
# that cannot run `scc check` is a container the gate cannot be verified in, which
# defeats the point of having one.
#
# The base is node-shaped whatever the project is written in, and that is about
# the *agent's* toolchain rather than the project's: three of the four tools below
# ship on npm, so an image without node cannot install them — and features cannot
# supply it either, since they are layered on after this file has already run.
# A Go or Python project installs its own toolchain at the bottom and is none the
# worse for the node underneath.
FROM mcr.microsoft.com/devcontainers/javascript-node:22

# The agent's own toolchain. Every one of these is named by something scc
# scaffolds, which is the whole reason they are here rather than left to taste:
# the rules answer questions with `scc`, the entry file tells the agent to prefix
# commands with `rtk`, and `code-search.md` tells it to ask `codegraph` before
# reading files. An agent inside a container missing them discovers that one
# failed command at a time and falls back to reading whole files — the exact cost
# this methodology exists to remove.
#
# As `node` rather than as root, because this image puts that user's global prefix
# (/usr/local/share/npm-global) on PATH and a root install would land somewhere
# the agent does not look.
RUN su node -c "npm install -g \
      @anthropic-ai/claude-code \
      @protonspy/scc \
      @colbymchenry/codegraph"

# `gh` — the delivery flow opens the pull request with it, and `scc validate --pr`
# reads the PR body through it.
RUN apt-get update \
 && apt-get install -y --no-install-recommends gh \
 && rm -rf /var/lib/apt/lists/*

# RTK is the slow layer: it is a Rust build, so this installs a toolchain, builds
# one binary and throws the toolchain away. Delete this block if you would rather
# have fast rebuilds than filtered command output — nothing else depends on it,
# and `scc launch` will simply report that rtk is not here.
#
# Built under /tmp and installed into /usr/local/bin rather than into a home
# directory: that way the binary is on PATH for every user and every kind of
# shell, instead of only an interactive one that has sourced a .bashrc.
RUN export CARGO_HOME=/tmp/cargo RUSTUP_HOME=/tmp/rustup \
 && curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
      | sh -s -- -y --profile minimal --no-modify-path \
 && "$CARGO_HOME/bin/cargo" install --git https://github.com/rtk-ai/rtk --locked \
 && install -m 0755 "$CARGO_HOME/bin/rtk" /usr/local/bin/rtk \
 && rm -rf "$CARGO_HOME" "$RUSTUP_HOME"

# <<< project toolchain — /scc-init fills this in >>>
# A Go project needs golang; a Python project needs python and its package
# manager; a Node project needs whatever the one above is not. Install what `scc
# check` runs, and nothing that only a human would want.
