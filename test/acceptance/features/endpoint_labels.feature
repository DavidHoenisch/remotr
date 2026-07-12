@os_OS-TQG-006
Feature: Endpoint label reporting

  @os_OS-TQG-006
  Scenario: Sync labels are visible to the operator
    Given an authenticated operator and enrolled agent "debian"
    When the agent Syncs labels "site=e2e-berlin,role=web"
    Then endpoint listing shows those labels
