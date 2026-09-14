# A starting point, not a base image — this file is yours from the moment it is
# written. scc scaffolds it once and never updates it, the way it seeds docs/.
#
# `/scc-init` adjusts it to this project: the language toolchain, the package
# manager, and whatever the delivery gate's commands actually need. A container
# that cannot run `scc check` is a container the gate cannot be verified in, which
# defeats the point of having one.
FROM mcr.microsoft.com/devcontainers/base:ubuntu

# The agent's own toolchain. Every one of these is named by something scc
# scaffolds, which is the whole reason they are here rather than left to taste:
# the rules answer questions with `scc`, the entry file tells the agent to prefix
# commands with `rtk`, and `code-search.md` tells it to ask `codegraph` before
# reading files. An agent inside a container missing them discovers that one
# failed command at a time and falls back to reading whole files — the exact cost
# this methodology exists to remove.
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
RUN su node -c "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
      | sh -s -- -y --profile minimal --no-modify-path" \
 && su node -c "\$HOME/.cargo/bin/cargo install --git https://github.com/rtk-ai/rtk --locked" \
 && su node -c "\$HOME/.rustup/toolchains/*/lib/rustlib/ 2>/dev/null; rm -rf \$HOME/.rustup" \
 && echo 'export PATH="$HOME/.cargo/bin:$PATH"' >> /home/node/.bashrc

# <<< project toolchain — /scc-init fills this in >>>
# A Go project needs golang; a Node project needs the right node; a Python project
# needs python and its package manager. Install what `scc check` runs, and nothing
# that only a human would want.
