@os_OS-TQG-006
Feature: Enrollment and Sync

  @os_OS-TQG-006
  Scenario: An enrolled agent receives an authenticated artifact
    Given an authenticated operator
    When the operator creates an enrollment token for "test-fleet"
    And an agent enrolls using that token
    Then the agent stores credentials
    When the enrolled agent Syncs
    Then it receives an authenticated artifact
