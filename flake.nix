{
  # Local stack via Nix — NEVER Docker for local (global rule 23).
  # Docker only for remote deploy (Fly.io). `nix run .#services` brings up
  # Postgres through process-compose/services-flake. Playout roda no browser;
  # broadcast por servidor (Liquidsoap/Icecast) foi descontinuado.
  description = "AutoDJ — tocador com mixagem de DJ: stack de dev local (Nix, sem Docker)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
    process-compose-flake.url = "github:Platonic-Systems/process-compose-flake";
    services-flake.url = "github:juspay/services-flake";
  };

  outputs = inputs @ { flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [ "aarch64-darwin" "x86_64-darwin" "x86_64-linux" "aarch64-linux" ];
      imports = [ inputs.process-compose-flake.flakeModule ];

      perSystem = { system, pkgs, ... }:
        {
          # `nix run .#services` — só Postgres (opcional/multiusuário).
          # Uso caseiro nem sobe DB: persistência local = SQLite por browser.
          process-compose."services" = {
            imports = [ inputs.services-flake.processComposeModules.default ];

            services.postgres."pg" = {
              enable = true;
              package = pkgs.postgresql_17;
              initialDatabases = [{ name = "autodj"; }];
              listen_addresses = "127.0.0.1";
              port = 5432;
            };
          };

          devShells.default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_23 # bump when 1.26 lands in nixpkgs; go.mod pins toolchain
              nodejs_22
              pnpm
              ffmpeg # ffprobe for metadata + artwork extract
              aubio # BPM/beat tracking (async analysis)
              keyfinder-cli # musical key (Camelot) detection
              audiowaveform # waveform peaks JSON precompute
              postgresql_17
              go-task
            ];
            shellHook = ''
              echo "AutoDJ dev shell — Nix (no Docker for local)."
              echo "  nix run .#services   → Postgres (opcional)"
              echo "  task serve           → server + web/out (playout no browser)"
            '';
          };
        };
    };
}
