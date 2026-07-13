@os_OS-LIA-011
Feature: Local administrator lifecycle

  @os_OS-LIA-011
  Scenario: A local administrator is provisioned and revoked declaratively
    Given a declarative M2 local-administrator configuration
    When the agent provisions the local administrator
    Then the local administrator has only Remotr-owned access
    When the agent revokes the local administrator
    Then the account and Remotr-owned access are absent
