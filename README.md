# Remotr

## Introduction
Remotr is an MDM, state management solution specifically designed for linux
systems. In contrast with some open source configuration management solutions
(such as ansible) which depend on SSH access into the target machine, remotr
does not require any open ports, or the installation of ZTNA solutions (though
you can use ZTNA). Remotr requires the installation of an agent binary onto
the machine. This agent runs as a system service on the target machine and will
occasionally phone home to the central service in order to report in and pick
up any new changes to state that must be applied. The agent will also report
in an drift in state so that admins have a clear view of potential issues on
the endpoint.

Application of desired state is atomic, this ensures that if a bad setting, or
error happens during the application of the desired state the computer will not
become unresponsive or unusable. State is defined as a collection of yaml/json
documents.


The following builtins allow for configuration of the system with the need to for
customer logic:

- Software management (via a package manager like apt, dnf, pacman)
- 



**State definition that installs a collection of applications**
```yaml
version: v1
lastModified: <timestamp here>
name: manage dev application
description: |
  configured required applications for dev machines
targetDistro: arch
targetArch:
 - ARM
 - X86
packages:
  - name: xargs
    present: true
    arch:
      - X86
    packageManager: pacman
  - name: nmap
    present: false
    arch:
      - X86
    packageManager: pacman
```

**State
definition that configures settings through adjusting config files**
```yaml
version: v1
lastModified: <timestamp here>
name: require pubkeyauth for ssh
description: |
  configures ssh security by requiring that authentication
  into machine requires the use of SSH keys not passwords.
  will also completely disable login as root
targetDistro: arch
targetArch:
 - ARM
 - X86
files:
  - name: disable root login
    path: /etc/ssh/sshd_config
    updateExisting: true
    withRegx: s/^#\?PermitRootLogin.*/PermitRootLogin no/
  - name: disable password auth
    path: /etc/ssh/sshd_config
    updateExisting: true
    withRegx: (?mi)^\s*PasswordAuthentication\s+no\s*$
  - name: require keyauth auth
    path: /etc/ssh/sshd_config
    updateExisting: true
    withRegx: (?mi)^(?!\s*#)\s*PubkeyAuthentication\s+yes\s*$
```
