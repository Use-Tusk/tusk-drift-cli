{
  description = "A very basic flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs =
    {
      self,
      nixpkgs,
    }:
    let
      x86-linux = "x86_64-linux";
      arm-linux = "aarch64-linux";
      arm-macos = "aarch64-darwin";

    in
    {
      devShell.aarch64-darwin =
        with nixpkgs.legacyPackages.aarch64-darwin;
        mkShell {
          buildInputs = [
            go
            gopls
            gofumpt
            gotools
            delve

            buf
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
          ];

          TUSK_AUTH0_CLIENT_ID = "L9gLDU0uTEEdSb0CWVA9TURyNgiFYuki";
          TUSK_AUTH0_AUDIENCE = "drift-cli";
        };
    };

}
