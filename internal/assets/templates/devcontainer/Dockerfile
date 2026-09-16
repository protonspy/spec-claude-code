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
# Pinned, and that is not tidiness. `npm install -g <pkg>` with no version is a
# layer whose hash never changes, so Docker serves the first build's answer for as
# long as the cache survives while the registry and this workspace both move on —
# and the rebuild that looks like it fixed that did not. Measured: an image rebuilt
# the same day carried scc v0.23.0 into a workspace whose settings.json registers
# the UserPromptSubmit hook v0.24.0 added, so every prompt of the session answered
# `unknown hook stage "user-prompt"`. Bumping an ARG is what invalidates the layer.
#
# SCC_VERSION is the one that has to be kept in step, because it is the only one
# of the three with a contract to this repository: scc scaffolds the harness hooks
# in the harness's own settings.json and this scc is what runs them, so a binary
# older than the scaffold is a vocabulary the container does not have. Keep it at
# or above the version that scaffolded the workspace. The other two answer to
# nobody here — Claude Code updates itself at runtime — and are pinned only so a
# rebuild is reproducible.
#
# As `node` rather than as root, because this image puts that user's global prefix
# (/usr/local/share/npm-global) on PATH and a root install would land somewhere
# the agent does not look.
ARG SCC_VERSION=0.25.0
ARG CLAUDE_CODE_VERSION=2.1.273
ARG CODEGRAPH_VERSION=1.6.0
RUN su node -c "npm install -g \
      @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION} \
      @protonspy/scc@${SCC_VERSION} \
      @colbymchenry/codegraph@${CODEGRAPH_VERSION}"

# Where the credential volume lands, created here rather than left to the mount.
# Docker copies an image path's content *and its ownership* into a fresh named
# volume only when that path already exists; mount onto a path the image never
# created and Docker creates it root-owned, so the agent's first write under it
# fails — measured, as an EACCES on the per-session directory it makes at every
# session start, with the login it could not store then reported back as "not
# logged in".
RUN mkdir -p /home/node/.claude && chown node:node /home/node/.claude

# `gh` — the delivery flow opens the pull request with it, and `scc validate --pr`
# reads the PR body through it.
RUN apt-get update \
 && apt-get install -y --no-install-recommends gh \
 && rm -rf /var/lib/apt/lists/*

# The workspace is a bind mount from the host, so its files arrive owned by a uid
# this container does not have, and git refuses a repository it thinks belongs to
# somebody else — the ownership check that exists because a differently-owned .git
# can carry a config or a hook that runs on the next ordinary git command
# (CVE-2022-24765). Scoped to where devcontainers mount a workspace and no further:
# this container forwards GH_TOKEN and mounts the agent's credential volume, so a
# bare `*` here would hand the next crafted .git in a dependency tree exactly those.
#
# Set in the image rather than in postCreateCommand, and that is forced from both
# ends: --global writes ~/.gitconfig, which is the read-only bind that carries your
# identity in ("Device or resource busy", which failed postCreate, `devcontainer up`
# and the whole sandbox, back onto the host), while --system writes /etc/gitconfig,
# which postCreate's non-root user cannot. The build runs as root and can.
#
# The path is a literal because ${containerWorkspaceFolder} is not resolved at build
# time. A workspace mounted somewhere else gets git's ordinary ownership message and
# one `git config --global --add safe.directory <path>` to run — visible, not silent.
RUN git config --system --add safe.directory '/workspaces/*'

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
