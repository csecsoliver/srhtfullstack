{ pkgs, ... }:

{
  # passwordless ssh server
  services.openssh = {
    enable = true;
    settings = {
      PermitRootLogin = "yes";
      PermitEmptyPasswords = true;
    };
  };
  security.pam.services.sshd.allowNullPassword = true;

  users = {
    mutableUsers = false;
    # build user
    extraUsers."build" = {
      isNormalUser = true;
      uid = 1000;
      extraGroups = [ "wheel" ];
      password = "";
    };
    users.root.password = "";
  };
  security.sudo.wheelNeedsPassword = false;
  nix.settings.trusted-users = [ "root" "build" ];

  # builds.sr.ht-image-specific network settings
  networking = {
    hostName = "build";
    dhcpcd.enable = false;
    defaultGateway.address = "10.0.2.2";
    usePredictableInterfaceNames = false; # so that we just get eth0 and not some weird id
    interfaces."eth0".ipv4.addresses = [{
      address = "10.0.2.15";
      prefixLength = 25;
    }];
    enableIPv6 = false;
    nameservers = [
      # OpenNIC anycast
      "134.195.4.2" # https://servers.opennicproject.org/edit.php?srv=ns4.any.dns.opennic.glue
      # Google as a fallback :(
      "8.8.8.8"
    ];
    firewall.allowedTCPPorts = [ 22 ]; # allow ssh
  };

  environment.systemPackages = with pkgs; [
    gitMinimal
    mercurial
    curl
    gnupg
  ];
}

