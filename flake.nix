{
  description = ''
    Elephant - a powerful data provider service and backend for building custom application launchers and desktop utilities.
  '';

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    systems.url = "github:nix-systems/default-linux";
  };

  outputs =
    {
      self,
      nixpkgs,
      systems,
      ...
    }:
    let
      inherit (nixpkgs) lib;
      eachSystem = f: lib.genAttrs (import systems) (system: f nixpkgs.legacyPackages.${system});
    in
    {
      formatter = eachSystem (pkgs: pkgs.alejandra);

      devShells = eachSystem (pkgs: {
        default = pkgs.mkShell {
          name = "elephant-dev-shell";
          inputsFrom = [ self.packages.${pkgs.stdenv.system}.elephant ];
          buildInputs = with pkgs; [
            go
            gcc
            protobuf
            protoc-gen-go
          ];
        };
      });

      packages = eachSystem (pkgs: let
        version = lib.trim (builtins.readFile ./cmd/elephant/version.txt);
        defaultGoArgs = {
          inherit version;
          src = ./.;
          vendorHash = "sha256-XYGh4ZXRly3MCPWse51eUMyjXqtHzKbPEyD0J8S/MDk=";
          # Share go modules between all builds
          overrideModAttrs = _: _: {
            name = "elephant-${version}-go-modules";
          };

          meta = with lib; {
            homepage = "https://github.com/abenz1267/elephant";
            license = licenses.gpl3Only;
            platforms = platforms.linux;
          };
        };
        buildGo = attrs: pkgs.buildGo125Module (lib.recursiveUpdate defaultGoArgs attrs);

        buildProviders = providersPath: let
          buildProvider = name: buildGo {
            pname = "elephant-provider-${name}";

            buildInputs = with pkgs; [
              wayland
            ];

            nativeBuildInputs = with pkgs; [
              protobuf
              protoc-gen-go
            ];

            buildPhase = ''
              runHook preBuild

              echo "Building elephant provider: ${name}"
              if ! go build -buildmode=plugin -o "${name}.so" ./${providersPath}/${name}; then
                echo "⚠ Failed to build provider: ${name}.so"
                exit 1
              fi
              echo "Built ${name}.so"

              runHook postBuild
            '';

            installPhase = ''
              runHook preInstall

              mkdir -p $out/lib/elephant/providers
              cp "${name}.so" "$out/lib/elephant/providers/"
              echo "Installed provider: ${name}.so"

              runHook postInstall
            '';

            meta.description = "Elephant ${name} provider";
          };

          dirHasGoFile = dir:
              builtins.any
                (lib.hasSuffix ".go")
                (builtins.attrNames
                  (builtins.readDir dir));

          providerDirs =
            lib.attrNames
              (lib.filterAttrs (name: type: type == "directory" && dirHasGoFile (./. + "/${providersPath}/${name}"))
                (builtins.readDir (./. + "/${providersPath}")));
        in
          lib.genAttrs'
            providerDirs
            (name: lib.nameValuePair "elephant-provider-${name}" (buildProvider name))
         ;
      in (buildProviders "internal/providers") // {
        default = self.packages.${pkgs.stdenv.system}.elephant-with-providers;

        # Main elephant binary
        elephant = buildGo {
          pname = "elephant";

          buildInputs = with pkgs; [
            protobuf
          ];

          nativeBuildInputs = with pkgs; [
            protoc-gen-go
            makeWrapper
          ];

          # Build from cmd/elephant/elephant.go
          subPackages = [
            "cmd/elephant"
          ];

          postFixup = ''
             wrapProgram $out/bin/elephant \
            	    --prefix PATH : ${lib.makeBinPath (with pkgs; [ fd ])}
          '';

          meta.description = "Powerful data provider service and backend for building custom application launchers";
        };


        elephant-providers = pkgs.symlinkJoin {
          pname = "elephant-providers";
          inherit version;
          paths = lib.attrValues (lib.filterAttrs (name: _: lib.hasPrefix "elephant-provider-" name) self.packages.${pkgs.stdenv.system});
        };

        # Combined package with elephant + providers
        elephant-with-providers = pkgs.stdenv.mkDerivation {
          pname = "elephant-with-providers";
          version = lib.trim (builtins.readFile ./cmd/elephant/version.txt);

          dontUnpack = true;

          buildInputs = [
            self.packages.${pkgs.stdenv.system}.elephant
            self.packages.${pkgs.stdenv.system}.elephant-providers
          ];

          nativeBuildInputs = with pkgs; [
            makeWrapper
          ];

          installPhase = ''
            mkdir -p $out/bin $out/lib/elephant
            cp ${self.packages.${pkgs.stdenv.system}.elephant}/bin/elephant $out/bin/
            cp -r ${
              self.packages.${pkgs.stdenv.system}.elephant-providers
            }/lib/elephant/providers $out/lib/elephant/
          '';

          postFixup = ''
            wrapProgram $out/bin/elephant \
                  --prefix PATH : ${
                    lib.makeBinPath (
                      with pkgs;
                      [
                        wl-clipboard
                        libqalculate
                        imagemagick
                        bluez
                      ]
                    )
                  }
          '';

          meta = with lib; {
            description = "Elephant with all providers (complete installation)";
            homepage = "https://github.com/abenz1267/elephant";
            license = licenses.gpl3Only;
            platforms = platforms.linux;
          };
        };
      });

      homeManagerModules = {
        default = self.homeManagerModules.elephant;
        elephant = import ./nix/modules/home-manager.nix self;
      };

      nixosModules = {
        default = self.nixosModules.elephant;
        elephant = import ./nix/modules/nixos.nix self;
      };
    };
}
