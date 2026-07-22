Feature: Capability-aware artifact delivery
  Remotr validates mixed-target configuration, upgrades blocked agents, and activates only acknowledged artifacts.

  @os_OS-AEC-104 @os_OS-AEC-108 @os_OS-AEC-109 @os_OS-AEC-110 @os_OS-LPC-027 @os_OS-UPM-062 @os_OS-UPM-064
  Scenario: Qualified Ubuntu endpoint recovers from a capability-blocked legacy agent
    Given the representative mixed-target Ubuntu Pro repository
    When the operator validates it and the server accepts the Git snapshot
    Then Ubuntu delivery is not rejected for the Arch package branch
    Given a legacy Ubuntu endpoint is explicitly targeted for the approved current agent
    When the legacy endpoint performs authenticated Sync
    Then artifact delivery is blocked and the approved agent upgrade is returned
    When the upgraded endpoint reports its current observed capabilities
    Then the complete Ubuntu artifact is offered without Pacman requirements
    When the endpoint acknowledges the exact offered digest
    Then fleet delivery state reports that digest active
