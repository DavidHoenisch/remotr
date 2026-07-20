package resourceregistry

import (
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
)

type fieldPolicy struct {
	public        []string
	metadata      []string
	counts        []string
	fingerprints  []string
	sensitiveOmit []string
	secretOmit    []string
	references    []string
	presence      []string
}

func explicitFieldDescriptors(kind models.ResourceKind) FieldDescriptors {
	fields := make(FieldDescriptors)
	addFieldDescriptors(fields, SensitivityPublic, ProjectValue, paths(`
		kind name lifecycle dependsOn[] policy ownership notifications[].type risk enforce lockDomains[]
	`))
	addFieldDescriptors(fields, SensitivitySensitiveMetadata, ProjectMetadata, paths(`
		authorizationGroup notifications[].target
	`))
	addFieldDescriptors(fields, SensitivitySecret, ProjectOmit, paths(`
		providerOptions.*.* validation[].command[] preApplyValidation[]
	`))

	policy, ok := explicitFieldPolicies[kind]
	if !ok {
		return fields
	}
	addFieldDescriptors(fields, SensitivityPublic, ProjectValue, policy.public)
	addFieldDescriptors(fields, SensitivitySensitiveMetadata, ProjectMetadata, policy.metadata)
	addFieldDescriptors(fields, SensitivitySensitiveMetadata, ProjectCount, policy.counts)
	addFieldDescriptors(fields, SensitivitySensitiveMetadata, ProjectFingerprint, policy.fingerprints)
	addFieldDescriptors(fields, SensitivitySensitiveMetadata, ProjectOmit, policy.sensitiveOmit)
	addFieldDescriptors(fields, SensitivitySecret, ProjectOmit, policy.secretOmit)
	addFieldDescriptors(fields, SensitivitySecret, ProjectReference, policy.references)
	addFieldDescriptors(fields, SensitivitySecret, ProjectPresence, policy.presence)
	return fields
}

func addFieldDescriptors(fields FieldDescriptors, sensitivity Sensitivity, projection SafeProjection, fieldPaths []string) {
	for _, path := range fieldPaths {
		if _, exists := fields[path]; exists {
			// Preserve a fail-closed marker so registration reports duplicate
			// policy assignment instead of accepting whichever entry was last.
			fields[path] = FieldDescriptor{}
			continue
		}
		fields[path] = FieldDescriptor{Sensitivity: sensitivity, Projection: projection}
	}
}

func paths(value string) []string { return strings.Fields(value) }

var explicitFieldPolicies = map[models.ResourceKind]fieldPolicy{
	models.ResourceKindPackage: {
		public: paths(`
			allowDowngrade allowUpgrade arch flatpakRemote hold nonInteractive packageManager present
			pwaBrowser pwaTitle refreshCache removeDependencies version
		`),
		metadata:   paths(`aurBuildUser pwaUsers`),
		secretOmit: paths(`flatpakRemoteURL pwaIcon pwaURL`),
	},
	models.ResourceKindAPTSigningKey: {
		fingerprints: paths(`fingerprint`),
		secretOmit:   paths(`source`),
	},
	models.ResourceKindAPTRepository: {
		public:     paths(`architectures[] components[] priority signingKey suites[]`),
		secretOmit: paths(`url`),
		references: paths(`credentialRef`),
	},
	models.ResourceKindPacmanSigningKey: {
		fingerprints: paths(`fingerprint`),
		secretOmit:   paths(`source`),
	},
	models.ResourceKindPacmanRepository: {
		public:     paths(`architecture signatureLevel signingKeys[]`),
		secretOmit: paths(`servers[]`),
		references: paths(`credentialRef`),
	},
	models.ResourceKindFile: {
		public:     paths(`mode[] updateExisting`),
		metadata:   paths(`group owner path`),
		secretOmit: paths(`content replaceRegx withRegx`),
	},
	models.ResourceKindDirectory: {
		public:   paths(`allowTypeReplacement crossFilesystem maxDepth maxEntries mode[] purge recursive`),
		metadata: paths(`group owner path`),
		counts:   paths(`exclusions[]`),
	},
	models.ResourceKindLink: {
		public:   paths(`allowTypeReplacement linkType`),
		metadata: paths(`group owner path target`),
	},
	models.ResourceKindSysctl: {
		public: paths(`activation key persistent runtime value`),
	},
	models.ResourceKindHostname: {
		metadata: paths(`static transient`),
	},
	models.ResourceKindHostLocale: {
		public: paths(`keymap locale.* timezone`),
	},
	models.ResourceKindTimeSync: {
		public: paths(`enabled provider`),
		counts: paths(`pools[] servers[]`),
	},
	models.ResourceKindDownload: {
		public:        paths(`checksum mode[] redirectPolicy timeout`),
		metadata:      paths(`dest group notifySystemd owner`),
		fingerprints:  paths(`trustedSigner`),
		sensitiveOmit: paths(`signature`),
		secretOmit:    paths(`reloadExec[] url`),
		references:    paths(`authenticationRef`),
	},
	models.ResourceKindKernelModule: {
		public: paths(`blacklisted loaded module parameters.* persistent protectedModules[]`),
	},
	models.ResourceKindMount: {
		public:     paths(`dump filesystemType mounted pass persistent unmountMode`),
		metadata:   paths(`source target`),
		secretOmit: paths(`options[]`),
	},
	models.ResourceKindSwap: {
		public:   paths(`active allowRemove persistent priority sizeBytes type`),
		metadata: paths(`path`),
	},
	models.ResourceKindGroup: {
		public:   paths(`allowGIDReassignment system`),
		metadata: paths(`gid group`),
	},
	models.ResourceKindUser: {
		public: paths(`
			allowUIDReassignment createHome forceRemoval locked present removeHome supplementaryGroupsMode system
		`),
		metadata:   paths(`comment expiry home primaryGroup shell uid username`),
		counts:     paths(`supplementaryGroups[]`),
		references: paths(`passwordHashRef`),
	},
	models.ResourceKindAuthorizedKey: {
		public:        paths(`entries[].expiresAt entries[].type`),
		metadata:      paths(`entries[].comment user`),
		counts:        paths(`entries[].principals[] entries[].restrictions[] recoveryPrincipals[]`),
		fingerprints:  paths(`entries[].fingerprint`),
		sensitiveOmit: paths(`entries[].key`),
	},
	models.ResourceKindKnownHost: {
		public:        paths(`hashing replaceExisting scope type`),
		metadata:      paths(`comment user`),
		counts:        paths(`hosts[]`),
		fingerprints:  paths(`fingerprint`),
		sensitiveOmit: paths(`key`),
	},
	models.ResourceKindUserFile: {
		public:     paths(`mode[] selector.mode updateExisting`),
		metadata:   paths(`path users`),
		counts:     paths(`selector.usernames[]`),
		secretOmit: paths(`content replaceRegx withRegx`),
	},
	models.ResourceKindDesktopSetting: {
		public:     paths(`level provider scope selector.mode value.type`),
		metadata:   paths(`key path schema`),
		counts:     paths(`selector.usernames[]`),
		secretOmit: paths(`value.value`),
	},
	models.ResourceKindSessionPolicy: {
		public: paths(`
			disableCommandLine disableLogout disableUserSwitching idleTimeoutSeconds lockDelaySeconds lockEnabled
			provider proxy.httpPort proxy.httpsPort proxy.mode selector.mode trustAnchors[]
		`),
		metadata: paths(`defaultApplications.* proxy.automaticUrl proxy.httpHost proxy.httpsHost`),
		counts:   paths(`proxy.ignoreHosts[] selector.usernames[]`),
	},
	models.ResourceKindBrowserPolicy: {
		public:     paths(`browser level policyName scope trustAnchors[] value.type`),
		secretOmit: paths(`value.value`),
	},
	models.ResourceKindSudo: {
		public: paths(`tags[]`),
		counts: paths(`commands[] recoveryPrincipals[] runAs[] subjects[]`),
	},
	models.ResourceKindFirewall: {
		public: paths(`
			action audit backend chain cleanupLimit family protectRemotr protocol rollbackTimeout
			rules[].action rules[].name rules[].protocol table
		`),
		counts: paths(`
			destinations[] ports[] rules[].destinations[] rules[].ports[] rules[].services[]
			rules[].sources[] services[] sources[] zones[]
		`),
		secretOmit: paths(`rule rules[].rule`),
	},
	models.ResourceKindHostsEntry: {
		metadata: paths(`address canonicalHost`),
		counts:   paths(`aliases[]`),
	},
	models.ResourceKindDNSResolver: {
		public:   paths(`configured effective provider`),
		metadata: paths(`interface`),
		counts:   paths(`searchDomains[] servers[]`),
	},
	models.ResourceKindRoute: {
		public:   paths(`configured effective metric provider table`),
		metadata: paths(`destination gateway interface`),
	},
	models.ResourceKindNetworkProfile: {
		public: paths(`
			audit autoConnect ipv4Method ipv6Method mtu profileType provider rollbackTimeout selector.type
		`),
		metadata:   paths(`profileName selector.name selector.permanentMAC ssid`),
		counts:     paths(`addresses[]`),
		references: paths(`credentialRef`),
	},
	models.ResourceKindSystemdUnit: {
		public:     paths(`dropIn mode[]`),
		metadata:   paths(`group owner unit`),
		secretOmit: paths(`content`),
	},
	models.ResourceKindCertificate: {
		public:       paths(`certificateMode[] privateKeyMode[] renewBefore renewalPolicy`),
		metadata:     paths(`certificatePath group owner privateKeyPath subject`),
		counts:       paths(`sans[]`),
		fingerprints: paths(`fingerprint`),
		references:   paths(`certificateRef chainRefs[] privateKeyRef`),
	},
	models.ResourceKindTrustAnchor: {
		fingerprints: paths(`fingerprint`),
		references:   paths(`anchorRef`),
	},
	models.ResourceKindAppArmorProfile: {
		public:     paths(`mode`),
		metadata:   paths(`profile`),
		secretOmit: paths(`content`),
	},
	models.ResourceKindAuditRules: {
		sensitiveOmit: paths(`rules[]`),
	},
	models.ResourceKindAccountLimit: {
		public:   paths(`entries[].item entries[].type`),
		metadata: paths(`entries[].domain entries[].value`),
	},
	models.ResourceKindLoginPolicy: {
		public:     paths(`priority provider rules[].control rules[].module rules[].section`),
		counts:     paths(`recoveryPrincipals[]`),
		secretOmit: paths(`rules[].arguments[]`),
	},
	models.ResourceKindEndpointSchedule: {
		public:     paths(`backend overlap persistent schedule shell timeout`),
		metadata:   paths(`environment[].name user workingDirectory`),
		secretOmit: paths(`argv[] environment[].value`),
		references: paths(`environment[].secretRef`),
	},
	models.ResourceKindSystemd: {
		public:   paths(`active enabled masked`),
		metadata: paths(`unit`),
	},
	models.ResourceKindService: {
		public:   paths(`active enabled linger masked provider scope`),
		metadata: paths(`service users`),
	},
	models.ResourceKindJournald: {
		public: paths(`
			forwardToConsole forwardToKernelBuffer forwardToSyslog forwardToWall maxRetention rateLimitBurst
			rateLimitInterval runtimeMaxUseBytes storage systemMaxUseBytes
		`),
	},
	models.ResourceKindLogrotate: {
		public:   paths(`cadence compress create.mode retention sharedScripts`),
		metadata: paths(`create.group create.owner`),
		counts:   paths(`paths[]`),
		secretOmit: paths(`
			firstAction.command[] lastAction.command[] postRotate.command[] preRotate.command[]
		`),
	},
	models.ResourceKindSystemdUser: {
		public:   paths(`active enabled linger masked`),
		metadata: paths(`unit unitPath users`),
	},
	models.ResourceKindReboot: {
		public: paths(`
			deadline delay generation maintenanceWindow.duration maintenanceWindow.start maintenanceWindow.weekdays[]
			onlyIfRequired requireACPower timeout userInhibition workloadInhibition
		`),
	},
	models.ResourceKindBootstrap: {
		public:     paths(`steps[].systemd.active steps[].systemd.enabled`),
		metadata:   paths(`steps[].systemd.unit when.pathExists when.pathMissing`),
		secretOmit: paths(`steps[].exec[]`),
	},
	models.ResourceKindAgentInstall: {
		public:     paths(`present version`),
		metadata:   paths(`extractDir installBinary runningCheck.process`),
		secretOmit: paths(`artifactURL fleetURL`),
		references: paths(`enrollmentTokenSecret`),
	},
	models.ResourceKindCommand: {
		secretOmit: paths(`apply[] check[] revert[]`),
	},
}
