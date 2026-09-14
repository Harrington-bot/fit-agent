# Auto-split elevation algorithm

Auto-split segment elevation is deliberately anchored to totals produced by
the recording device. Raw GPS altitude is useful for *where* a route climbed
or descended, but not for its absolute gain/loss: drift and vertical noise can
make a flat route appear hilly.

## Inputs and trust boundary

- FIT lap `TotalAscent` and `TotalDescent` are the authoritative totals when
  present. Garmin and comparable devices apply their own sensor-aware filters.
- FIT records provide only the relative climb/descent shape between segments.
- A session total is used only when a lap has no corresponding total. It is
  divided between laps in proportion to lap distance, then between the lap's
  auto-splits. Raw record totals are never emitted as elevation values.

## Algorithm

For each auto-split lap:

1. Collect valid altitude records belonging to each segment.
2. Apply a 3 m hysteresis filter within each segment to obtain unscaled gain
   and loss weights. This rejects small sensor fluctuations while retaining
   the relative terrain shape.
3. Scale positive segment gain weights to sum exactly to the lap's filtered
   ascent total; scale loss weights independently to sum exactly to the lap's
   filtered descent total.
4. If no segment has a usable shape (or there are no records), distribute the
   authoritative total by segment distance.
5. If neither the lap nor session reports a total, omit auto-split elevation.

Consequently, the displayed auto-split gain/loss sums to the FIT device total
(up to normal one-decimal rendering), preventing GPS drift from being
mistaken for climbing.

## Edge cases

- **No altitude records, but a FIT total:** distance-proportional allocation
  preserves the device total without inventing terrain detail.
- **No FIT elevation total:** elevation is omitted rather than estimated from
  untrusted raw GPS altitude.
- **Multi-lap activities:** a missing lap total receives only its
  distance-proportional share of the session total, avoiding duplicated totals.
- **Remainder segments:** participate normally and receive their distance or
  shape-proportional share.

`auto_split_distance` controls segment length (`1km` by default, `none` to
disable it). The configuration changes only presentation; raw cache files are
unchanged.
