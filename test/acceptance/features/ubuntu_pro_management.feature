Feature: Ubuntu Pro management
  Remotr manages Ubuntu Pro through typed desired state and credential-free qualification.

  @os_OS-UPM-010 @os_OS-UPM-017 @os_OS-UPM-025
  Scenario: Attachment ordinary service resync and explicit detach
    Given an authenticated Ubuntu Pro workflow with an active synthetic token
    When the exact Ubuntu endpoint Syncs and applies attachment
    Then fleet state reports attached without exposing the token
    When the operator adds the ordinary service "esm-apps"
    Then the service converges and an idempotent resync makes no change
    When the operator explicitly detaches Ubuntu Pro
    Then fleet state reports detached

  @os_OS-UPM-049
  Scenario: Dependencies and conflicts are planned before mutation
    Given an authenticated pre-attached Ubuntu Pro workflow with FIPS enabled
    When the operator replaces FIPS with Livepatch
    Then the owned conflict is disabled before Livepatch is enabled

  @os_OS-UPM-060
  Scenario: Unobservable specialized mode is capability blocked
    Given an authenticated pre-attached Ubuntu Pro workflow
    When the operator requests access-only ESM Apps
    Then authenticated Sync blocks the unadvertised specialized capability

  @os_OS-UPM-051
  Scenario: Boot-impact service reports reboot without executing it
    Given an authenticated pre-attached Ubuntu Pro workflow
    When the operator enables FIPS through the mock API
    Then fleet state reports reboot required without rebooting
