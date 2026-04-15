package contract

import "sort"

func ValidateSemantics(cfg *Contract) error {
	if cfg == nil {
		return nil
	}

	labels := make([]string, 0, len(cfg.Identity))
	for label := range cfg.Identity {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	for _, label := range labels {
		spec := cfg.Identity[label]
		declared, ok := cfg.Services[spec.Service]
		if !ok {
			continue
		}
		if declared.State == ServiceStateActive {
			continue
		}

		return errorAt(
			[]string{"identity", label, "service"},
			"references %s but services.%s.state is %s; runtime identity requires active",
			spec.Service,
			spec.Service,
			declared.State,
		)
	}

	return nil
}
