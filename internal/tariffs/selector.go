package tariffs

func SelectActiveElements(t Tariff, snap Snapshot) map[TariffDimensionType]TariffElement {
	selected := make(map[TariffDimensionType]TariffElement)

	for _, element := range t.Elements {
		if !Matches(element.Restrictions, snap) {
			continue
		}

		for _, component := range element.PriceComponents {
			if _, exists := selected[component.Type]; exists {
				continue
			}
			selected[component.Type] = element
		}
	}

	return selected
}
