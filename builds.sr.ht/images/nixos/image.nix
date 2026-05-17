{ pkgs ? import <nixpkgs> { }
, hostPlatform ? { system = builtins.currentSystem; }
}:

let
  makeDiskImage = import "${pkgs.path}/nixos/lib/make-disk-image.nix";
  evalConfig = import "${pkgs.path}/nixos/lib/eval-config.nix";
  config = (evalConfig {
    system = null; # Pass system parameters modularly
    modules = [
      (import ./qemu-system-configuration.nix)
      ({ ... }: { nixpkgs.hostPlatform = hostPlatform; })
    ];
  }).config;
in
  makeDiskImage {
    inherit pkgs config;
    lib = pkgs.lib;
    diskSize = 16000;
    format = "qcow2-compressed";
    contents = [{
      source = pkgs.writeText "gitconfig" ''
        [user]
          name = builds.sr.ht
          email = builds@sr.ht
      '';
      target = "/home/build/.gitconfig";
      user = "build";
      group = "users";
      mode = "644";
    }];
  }

