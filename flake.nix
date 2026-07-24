{
  description = "Unofficial command-line client for the Proton suite (Mail, Drive, Calendar, Contacts, Pass)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-darwin"
        "x86_64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      version = self.shortRev or self.dirtyShortRev or "dev";
    in
    {
      packages = forAllSystems (pkgs: {
        default = pkgs.buildGoModule {
          pname = "proton-cli";
          inherit version;

          __structuredAttrs = true;

          src = self;
          vendorHash = "sha256-bFpBsxU9dehg4X5xBjzb8es7S+RdnTeHDiqlUM1kIuY=";

          subPackages = [ "." ];
          tags = [ "embed_hv" ];

          ldflags = [
            "-s"
            "-w"
            "-X=github.com/roman-16/proton-cli/internal/cli.version=${version}"
          ];

          nativeBuildInputs = [
            pkgs.installShellFiles
            pkgs.pkg-config
          ];

          buildInputs = pkgs.lib.optionals pkgs.stdenv.hostPlatform.isLinux [
            pkgs.webkitgtk_4_1
            pkgs.gtk3
          ];

          # Builds the embedded CAPTCHA webview helper for the host platform,
          # matching the release binaries and the nixpkgs package.
          preBuild = ''
            bash scripts/build-hv-helpers.sh
          '';

          overrideModAttrs = _: {
            preBuild = null;
          };

          postInstall = ''
            installShellCompletion --cmd proton-cli \
              --bash <($out/bin/proton-cli completion bash) \
              --fish <($out/bin/proton-cli completion fish) \
              --zsh  <($out/bin/proton-cli completion zsh)
          '';

          meta = {
            description = "Unofficial command-line client for the Proton suite (Mail, Drive, Calendar, Contacts, Pass)";
            homepage = "https://github.com/roman-16/proton-cli";
            license = pkgs.lib.licenses.mit;
            mainProgram = "proton-cli";
            platforms = pkgs.lib.platforms.linux ++ pkgs.lib.platforms.darwin;
          };
        };
      });

      apps = forAllSystems (pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${pkgs.stdenv.hostPlatform.system}.default}/bin/proton-cli";
          meta.description = "Unofficial command-line client for the Proton suite";
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
