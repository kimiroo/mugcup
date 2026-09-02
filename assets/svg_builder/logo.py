import math

def gen_3d_mug_svg(is_empty: bool, shadow_type: str) -> str:
    def arc_beziers(cx, cy, rx, ry, t1, t2, nseg=None):
        span = t2 - t1
        if nseg is None:
            nseg = max(1, int(math.ceil(abs(span) / (math.pi / 2 - 1e-9))))
        out = []
        for i in range(nseg):
            a = t1 + span * i / nseg
            b = t1 + span * (i + 1) / nseg
            alpha = 4.0 / 3.0 * math.tan((b - a) / 4.0)
            p0 = (cx + rx * math.cos(a), cy + ry * math.sin(a))
            p3 = (cx + rx * math.cos(b), cy + ry * math.sin(b))
            d0 = (-rx * math.sin(a), ry * math.cos(a))
            d3 = (-rx * math.sin(b), ry * math.cos(b))
            p1 = (p0[0] + alpha * d0[0], p0[1] + alpha * d0[1])
            p2 = (p3[0] - alpha * d3[0], p3[1] - alpha * d3[1])
            out.append((p1, p2, p3))
        return out

    def f(v):
        return ('%.3f' % v).rstrip('0').rstrip('.')

    def seg_str(segs):
        return ' '.join('C %s,%s %s,%s %s,%s' % (f(p1[0]), f(p1[1]), f(p2[0]), f(p2[1]), f(p3[0]), f(p3[1]))
                        for p1, p2, p3 in segs)

    CX, RIM_CY, RIM_RX, RIM_RY = 477.34, 386.647, 200, 55.0
    _scale = RIM_RY / 74.143
    IN_RX, IN_RY = 163.0, 47.0 * _scale
    CONT_CY = RIM_CY + (425.621 - 386.647) * _scale
    WALL_L, WALL_R = CX - RIM_RX, CX + RIM_RX
    WALL_TOP, WALL_BOT = RIM_CY, 700
    BASE_RY = 85
    BASE_BOT = WALL_BOT + BASE_RY

    def solve_lens(A2, B2, ACY, C2, D2, CCY):
        a = -A2 / B2 + C2 / D2
        b = 2 * A2 * ACY / B2 - 2 * C2 * CCY / D2
        c = A2 - A2 * ACY**2 / B2 - C2 + C2 * CCY**2 / D2
        if abs(a) < 1e-9:
            y = -c / b
        else:
            disc = b * b - 4 * a * c
            y1 = (-b + math.sqrt(disc)) / (2 * a)
            y2 = (-b - math.sqrt(disc)) / (2 * a)
            y = y1 if ACY - math.sqrt(B2) < y1 < ACY + math.sqrt(B2) else y2
        u = math.sqrt(max(0.0, A2 * (1 - (y - ACY)**2 / B2)))
        return y, u

    LY, LU = solve_lens(IN_RX**2, IN_RY**2, CONT_CY, IN_RX**2, IN_RY**2, RIM_CY)
    tl_c = math.atan2((LY - CONT_CY) / IN_RY, -LU / IN_RX)
    if tl_c > 0: tl_c -= 2 * math.pi
    tr_c = math.atan2((LY - CONT_CY) / IN_RY, LU / IN_RX)
    upper = arc_beziers(CX, CONT_CY, IN_RX, IN_RY, tl_c, tr_c)
    tr_o = math.atan2((LY - RIM_CY) / IN_RY, LU / IN_RX)
    tl_o = math.pi - tr_o
    lower = arc_beziers(CX, RIM_CY, IN_RX, IN_RY, tr_o, tl_o)
    content_d = ('M %s,%s ' % (f(CX - LU), f(LY))) + seg_str(upper) + ' ' + seg_str(lower) + ' Z'

    k = 0.5522847498
    base_l = 'C %s,%s %s,%s %s,%s' % (f(WALL_L), f(WALL_BOT + k * BASE_RY), f(CX - k * RIM_RX), f(BASE_BOT), f(CX), f(BASE_BOT))
    base_r = 'C %s,%s %s,%s %s,%s' % (f(CX + k * RIM_RX), f(BASE_BOT), f(WALL_R), f(WALL_BOT + k * BASE_RY), f(WALL_R), f(WALL_BOT))
    cup_lower_d = 'M %s,%s L %s,%s %s %s L %s,%s' % (f(WALL_L), f(WALL_TOP), f(WALL_L), f(WALL_BOT), base_l, base_r, f(WALL_R), f(WALL_TOP))

    HX, HY = WALL_R - 1, (WALL_TOP + WALL_BOT) / 2 + 5
    TH, RC, FRO, FRI = 50.0, 100.0, 14.0, 10.0
    RO, RI = RC + TH / 2, RC - TH / 2

    dy_i = math.sqrt(RI * RI - 2 * RI * FRI)
    fc_it, fc_ib = (HX + FRI, HY - dy_i), (HX + FRI, HY + dy_i)
    Li = math.hypot(FRI, dy_i)
    tan_it = (HX + RI * (FRI / Li), HY - RI * (dy_i / Li))
    tan_ib = (HX + RI * (FRI / Li), HY + RI * (dy_i / Li))
    th_in = math.atan2(tan_it[1] - HY, tan_it[0] - HX)
    inner = arc_beziers(HX, HY, RI, RI, -th_in, th_in)
    fil_ib = arc_beziers(fc_ib[0], fc_ib[1], FRI, FRI, math.pi, math.atan2(tan_ib[1] - fc_ib[1], tan_ib[0] - fc_ib[0]))
    fil_it = arc_beziers(fc_it[0], fc_it[1], FRI, FRI, math.atan2(tan_it[1] - fc_it[1], tan_it[0] - fc_it[0]), -math.pi)

    dy_o = math.sqrt(RO * RO + 2 * RO * FRO)
    o_top, o_bot = HY - dy_o, HY + dy_o
    fc_ot, fc_ob = (HX + FRO, o_top), (HX + FRO, o_bot)
    Lo = math.hypot(FRO, dy_o)
    tan_ot = (HX + RO * (FRO / Lo), HY - RO * (dy_o / Lo))
    tan_ob = (HX + RO * (FRO / Lo), HY + RO * (dy_o / Lo))
    th_out = math.atan2(tan_ot[1] - HY, tan_ot[0] - HX)
    flare_t = arc_beziers(fc_ot[0], fc_ot[1], FRO, FRO, math.pi, math.atan2(tan_ot[1] - fc_ot[1], tan_ot[0] - fc_ot[0]))
    outer = arc_beziers(HX, HY, RO, RO, th_out, -th_out)
    flare_b = arc_beziers(fc_ob[0], fc_ob[1], FRO, FRO, math.atan2(tan_ob[1] - fc_ob[1], tan_ob[0] - fc_ob[0]), -math.pi)

    handle_d = ('M %s,%s ' % (f(HX), f(o_top)) + seg_str(flare_t) + ' ' + seg_str(outer) + ' ' + seg_str(flare_b)
                + ' L %s,%s ' % (f(HX), f(HY + dy_i)) + seg_str(fil_ib) + ' ' + seg_str(inner) + ' ' + seg_str(fil_it) + ' Z')

    content_elements = f'''    <path d="{content_d}" fill="url(#coffeeGrad)"/>
    <ellipse cx="{f(CX - 35)}" cy="{f(RIM_CY + 12)}" rx="{f(IN_RX * 0.45)}" ry="{f(IN_RY * 0.35)}" fill="#FFFFFF" opacity="0.16" transform="rotate(-8 {f(CX - 35)} {f(RIM_CY + 12)})" />''' if not is_empty else ''

    # ---- shadow definitions (tuned) ----
    if shadow_type == 'heavy':  # EXE icon: darker shadow, and MORE padding so it never clips
        shadow_def = '<filter id="outerDropShadow" x="-60%" y="-60%" width="220%" height="220%"><feDropShadow dx="0" dy="14" stdDeviation="18" flood-color="#000000" flood-opacity="0.85" /></filter>'
        group_tag = '<g filter="url(#outerDropShadow)">'
        # more padding around the cup (was 255 275 560 560 -> icon touches/overlaps shadow at edges)
        viewbox = "215 245 640 640"
    else:                       # Logo: stronger shadow AND icon drawn bigger (tighter crop)
        shadow_def = '''<filter id="outerDropShadow" x="-50%" y="-50%" width="200%" height="200%">
          <feGaussianBlur in="SourceAlpha" stdDeviation="26" result="blurWide"/>
          <feOffset in="blurWide" dx="0" dy="14" result="offsetWide"/>
          <feComponentTransfer in="offsetWide"><feFuncA type="linear" slope="0.55"/></feComponentTransfer>
          <feMerge><feMergeNode/><feMergeNode in="SourceGraphic"/></feMerge>
        </filter>'''
        group_tag = '<g filter="url(#outerDropShadow)">'
        # tighter crop = bigger icon (was 160 175 760 760)
        viewbox = "210 225 640 640"

    return f'''<?xml version="1.0" encoding="utf-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="{viewbox}" width="1024px" height="1024px">
  <defs>
    {shadow_def}
    <clipPath id="cupBodyClip"><path d="{cup_lower_d}" /></clipPath>
    <clipPath id="handleClip"><path d="{handle_d}" /></clipPath>
    <linearGradient id="bodyGrad" x1="0%" y1="0%" x2="100%" y2="0%">
      <stop offset="0%" stop-color="#E1E6EB"/><stop offset="20%" stop-color="#FFFFFF"/>
      <stop offset="45%" stop-color="#F4F7F9"/><stop offset="80%" stop-color="#E2E7EC"/><stop offset="100%" stop-color="#CBD2DA"/>
    </linearGradient>
    <linearGradient id="bodyVerticalShadow" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#FFFFFF" stop-opacity="0.4"/>
      <stop offset="70%" stop-color="#FFFFFF" stop-opacity="0"/>
      <stop offset="100%" stop-color="#808C99" stop-opacity="0.18"/>
    </linearGradient>
    <linearGradient id="rimGrad" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#FFFFFF"/><stop offset="40%" stop-color="#F8FAFC"/>
      <stop offset="80%" stop-color="#E0E6EC"/><stop offset="100%" stop-color="#C5CBD3"/>
    </linearGradient>
    <linearGradient id="innerDepthGrad" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#929CA8"/><stop offset="40%" stop-color="#A7B1BC"/><stop offset="100%" stop-color="#BEC7D3"/>
    </linearGradient>
    <radialGradient id="coffeeGrad" cx="38%" cy="35%" r="65%">
      <stop offset="0%" stop-color="#7A3F28"/><stop offset="50%" stop-color="#4E281A"/>
      <stop offset="85%" stop-color="#2E170F"/><stop offset="100%" stop-color="#1B0C07"/>
    </radialGradient>
    <linearGradient id="handleGrad" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#FFFFFF"/><stop offset="30%" stop-color="#EBF0F5"/>
      <stop offset="70%" stop-color="#C8CFD7"/><stop offset="100%" stop-color="#A2AAB4"/>
    </linearGradient>
    <radialGradient id="handleSelfShadow" cx="60%" cy="65%" r="55%">
      <stop offset="50%" stop-color="#000000" stop-opacity="0"/>
      <stop offset="85%" stop-color="#55606E" stop-opacity="0.12"/><stop offset="100%" stop-color="#3A434E" stop-opacity="0.22"/>
    </radialGradient>
    <linearGradient id="handleInsideShadow" x1="100%" y1="0%" x2="0%" y2="0%">
      <stop offset="0%" stop-color="#000000" stop-opacity="0.08"/>
      <stop offset="8%" stop-color="#000000" stop-opacity="0.02"/><stop offset="20%" stop-color="#000000" stop-opacity="0"/>
    </linearGradient>
    <filter id="softBlur" x="-20%" y="-20%" width="140%" height="140%"><feGaussianBlur stdDeviation="6"/></filter>
  </defs>
  {group_tag}
    <path d="{handle_d}" fill="url(#handleGrad)"/>
    <path d="{handle_d}" fill="url(#handleSelfShadow)" clip-path="url(#handleClip)" />
    <path d="{handle_d}" fill="none" stroke="#FFFFFF" stroke-width="2" opacity="0.6" clip-path="url(#handleClip)" />
    <path d="{cup_lower_d}" fill="url(#bodyGrad)"/>
    <path d="{cup_lower_d}" fill="url(#bodyVerticalShadow)"/>
    <path d="{cup_lower_d}" fill="url(#handleInsideShadow)" clip-path="url(#cupBodyClip)"/>
    <path d="M {f(WALL_L + 28)},{f(WALL_TOP + 10)} Q {f(WALL_L + 35)},{f((WALL_TOP + WALL_BOT)/2)} {f(WALL_L + 28)},{f(WALL_BOT - 10)}" fill="none" stroke="#FFFFFF" stroke-width="14" stroke-linecap="round" opacity="0.3" filter="url(#softBlur)" clip-path="url(#cupBodyClip)"/>
    <ellipse cx="{f(CX)}" cy="{f(RIM_CY)}" rx="{f(RIM_RX)}" ry="{f(RIM_RY)}" fill="url(#rimGrad)"/>
    <ellipse cx="{f(CX)}" cy="{f(RIM_CY)}" rx="{f(IN_RX)}" ry="{f(IN_RY)}" fill="url(#innerDepthGrad)"/>
    {content_elements}
  </g>
</svg>'''

open('exe_icon.svg', 'w').write(gen_3d_mug_svg(is_empty=False, shadow_type='heavy'))
open('app_logo_github.svg', 'w').write(gen_3d_mug_svg(is_empty=False, shadow_type='soft'))
print("done")