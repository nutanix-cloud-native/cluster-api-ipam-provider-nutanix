{
  description = "Useful flakes for golang and Kubernetes projects";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = inputs @ { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      with nixpkgs.legacyPackages.${system}; rec {
        packages = rec {
          release-please = buildNpmPackage rec {
            pname = "release-please";
            version = "16.18.0";
            src = fetchFromGitHub {
              owner = "googleapis";
              repo = "release-please";
              rev = "v${version}";
              hash = "sha256-iY1EblSMCvw6iy8DFJnQRNCST7wycWSV8vdsq+XNpRU=";
            };
            npmDepsHash = "sha256-HDi7dFG/jNsszyvrb7ravVKQ7XO7NegnbX9MITcS1eE=";
            dontNpmBuild = true;
          };
        };

        formatter = alejandra;
      }
    );
}
