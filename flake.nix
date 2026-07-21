{
  description = "inc: a CLI for the incident.io API";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = self.shortRev or self.dirtyShortRev or "dev";
        inc = pkgs.buildGoModule {
          pname = "inc";
          inherit version;

          src = self;

          vendorHash = "sha256-BONInvPf9C1GAPYBxWwd8kTsNAPTX3pGnaQ0vjXz+EI=";

          ldflags = [
            "-s"
            "-w"
            "-X github.com/incident-io/inc/cmd.version=${version}"
          ];

          meta = {
            description = "CLI for incident.io";
            homepage = "https://incident.io/";
            license = pkgs.lib.licenses.mit;
            mainProgram = "inc";
          };
        };
      in
      {
        packages.inc = inc;
        packages.default = inc;
      });
}
