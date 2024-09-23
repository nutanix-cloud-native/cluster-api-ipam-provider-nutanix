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
          govulncheck = pkgs.govulncheck.override { buildGoModule = buildGo123Module; };

          setup-envtest = buildGo123Module rec {
            name = "setup-envtest";
            version = "0.19.0";
            src = fetchFromGitHub {
              owner = "kubernetes-sigs";
              repo = "controller-runtime";
              rev = "v${version}";
              hash = "sha256-9AqZMiA+OIJD+inmeUc/lq57kV7L85jk1I4ywiSKirg=";
            } + "/tools/setup-envtest";
            doCheck = false;
            subPackages = [ "." ];
            vendorHash = "sha256-sn3HiKTpQzjrFTOVOGFJwoNpxU+XWgkWD2EOcPilePY=";
            ldflags = [ "-s" "-w" ];
          };

          release-please = buildNpmPackage rec {
            pname = "release-please";
            version = "16.12.0";
            src = fetchFromGitHub {
              owner = "googleapis";
              repo = "release-please";
              rev = "v${version}";
              hash = "sha256-M4wsk0Vpkl6JAOM2BdSu8Uud7XA+iRHAaQOxHLux+VE=";
            };
            npmDepsHash = "sha256-UXWzBUrZCIklITav3VShL+whiWmvLkFw+/i/k0s13k0=";
            dontNpmBuild = true;
          };

          controller-gen = buildGo123Module rec {
            name = "controller-gen";
            version = "0.16.3";
            src = fetchFromGitHub {
              owner = "kubernetes-sigs";
              repo = "controller-tools";
              rev = "v${version}";
              hash = "sha256-Txvzp8OcRTDCAB8nFrqj93X+Kk/sNPSSLOI07J3DwcM=";
            };
            doCheck = false;
            subPackages = [ "./cmd/controller-gen" ];
            vendorHash = "sha256-nwzXlsSG7JF145bf/AJZB1GbGJRHJC7Q73Jty6mHc/w=";
            ldflags = [ "-s" "-w" ];
          };

          helm-schema = buildGo123Module rec {
            pname = "helm-schema";
            version = "1.5.3";

            src = fetchFromGitHub {
              owner = "losisin";
              repo = "helm-values-schema-json";
              rev = "v${version}";
              hash = "sha256-xKEJrNONB+781L1pdRE0EKV+5t/SAQxiKoOahUdjFS8=";
            };
            doCheck = false;
            vendorHash = "sha256-F2mT36aYkLjUZbV5GQH8mNMZjGi/70dTENU2rRhAJq4=";
            ldflags = let t = "main"; in [
              "-s"
              "-w"
              "-X ${t}.BuildDate=19700101-00:00:00"
              "-X ${t}.GitCommit=v${version}"
              "-X ${t}.Version=v${version}"
            ];

            postPatch = ''
              sed -i '/^hooks:/,+2 d' plugin.yaml
              sed -i 's#command: "$HELM_PLUGIN_DIR/schema"#command: "$HELM_PLUGIN_DIR/helm-values-schema-json"#' plugin.yaml
            '';

            postInstall = ''
              install -dm755 $out/${pname}
              mv $out/bin/* $out/${pname}/
              install -m644 -Dt $out/${pname} plugin.yaml
            '';
          };

          helm-with-plugins = wrapHelm kubernetes-helm {
            plugins = [
              helm-schema
            ];
          };
        };

        formatter = alejandra;
      }
    );
}
