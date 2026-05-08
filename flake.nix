{
  description = "Development environment for talos-state-metrics";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
      in
      {
        devShells.default = pkgs.mkShell {
          nativeBuildInputs = with pkgs; [
            # Go development
            go
            gopls
            gotools

            # Infrastructure & Cloud
            terraform
            infracost
            awscli2

            # Kubernetes & Talos
            talosctl
            kubectl
            kubernetes-helm
            yq-go

            # Utilities
            jq
          ];

          shellHook = ''
            echo "--- Talos State Metrics Dev Shell ---"
            echo "Go: $(go version)"
            echo "Terraform: $(terraform version -json | jq -r .terraform_version)"
            echo "Talosctl: $(talosctl version --client --short 2>/dev/null || echo 'not installed')"
            echo "------------------------------------"
            echo "To run infra tests: ./infra-test/run-test.sh"
          '';
        };
      }
    );
}
