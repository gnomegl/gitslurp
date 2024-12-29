package scanner

import (
	"context"
	"strings"

	"github.com/trufflesecurity/trufflehog/v3/pkg/detectors"
	"github.com/trufflesecurity/trufflehog/v3/pkg/pb/detectorspb"
)

func CheckForSecrets(content string) []string {
	var secrets []string
	ctx := context.Background()

	allDetectors := detectors.AllDetectors(ctx)

	for _, detector := range allDetectors {
		if detector.Type() == detectorspb.DetectorType_Network {
			continue
		}

		results, err := detector.FromData(ctx, []byte(content))
		if err != nil {
			continue
		}

		for _, result := range results {
			secret := strings.TrimSpace(result.Raw)
			if secret != "" {
				secrets = append(secrets, result.DetectorType.String()+": "+secret)
			}
		}
	}

	return secrets
}
