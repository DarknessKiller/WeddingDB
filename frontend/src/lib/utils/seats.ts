/**
 * Lowest free seat where a party of `pax` fits, or one past capacity when
 * nothing fits. Pure so the drawer's auto-select rule can be tested without
 * mounting the component.
 */
export function firstFittingSeat(available: number[], occupied: Set<number>, pax: number, capacity: number): number {
	for (const seat of available) {
		let fits = true;
		for (let k = 1; k < pax; k++) {
			if (occupied.has(seat + k) || seat + k > capacity) { fits = false; break; }
		}
		if (fits) return seat;
	}
	return capacity > 0 ? capacity + 1 : 1;
}
