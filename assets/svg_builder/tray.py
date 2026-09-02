import math

def gen_tray_svg_final(mode: str) -> str:
    def f(v): return ('%.3f' % v).rstrip('0').rstrip('.')
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
    def seg_str(segs):
        return ' '.join('C %s,%s %s,%s %s,%s' % (f(p1[0]), f(p1[1]), f(p2[0]), f(p2[1]), f(p3[0]), f(p3[1])) for p1, p2, p3 in segs)

    # --- Constants ---
    CX = 477.34
    RIM_CY = 386.647
    RIM_RX = 200
    WALL_L = CX - RIM_RX
    WALL_R = CX + RIM_RX
    WALL_TOP = RIM_CY
    WALL_BOT = 735
    BASE_RY = 85
    BASE_BOT = WALL_BOT + BASE_RY
    CORNER_R = 60

    cup_body_d = (
        f"M {f(WALL_L)},{f(WALL_TOP)} "
        f"L {f(WALL_R)},{f(WALL_TOP)} "
        f"L {f(WALL_R)},{f(BASE_BOT - CORNER_R)} "
        f"A {f(CORNER_R)},{f(CORNER_R)} 0 0 1 {f(WALL_R - CORNER_R)},{f(BASE_BOT)} "
        f"L {f(WALL_L + CORNER_R)},{f(BASE_BOT)} "
        f"A {f(CORNER_R)},{f(CORNER_R)} 0 0 1 {f(WALL_L)},{f(BASE_BOT - CORNER_R)} "
        "Z"
    )

    straight_mid = (WALL_TOP + BASE_BOT - CORNER_R) / 2
    y_offset = 12
    HX = WALL_R - 1
    HY = straight_mid + y_offset
    TH = 38.0; RC = 118.0; FRO = 14.0; FRI = 10.0
    RO = RC + TH / 2; RI = RC - TH / 2

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

    handle_d = (
        'M %s,%s ' % (f(HX), f(o_top))
        + seg_str(flare_t) + ' ' + seg_str(outer) + ' ' + seg_str(flare_b)
        + ' L %s,%s ' % (f(HX), f(HY + dy_i))
        + seg_str(fil_ib) + ' ' + seg_str(inner) + ' ' + seg_str(fil_it)
        + ' Z'
    )

    # --- Colors & Styles ---
    OUTLINE_COLOR = "#1C2026"
    OUTLINE_WIDTH = 36

    if mode == 'inactive':
        grad_stops = '''
            <stop offset="0%" stop-color="#D1D8DF"/>
            <stop offset="100%" stop-color="#C6CDD6"/>
        '''
        badge_elements = ""
        vertical_shadow_opacity = "0.15"
    else:
        grad_stops = '''
            <stop offset="0%" stop-color="#F2F5F8"/>
            <stop offset="25%" stop-color="#FFFFFF"/>
            <stop offset="60%" stop-color="#FAFCFD"/>
            <stop offset="85%" stop-color="#EDF1F5"/>
            <stop offset="100%" stop-color="#DFE4EA"/>
        '''
        vertical_shadow_opacity = "0.08"
        BCX = CX
        BCY = (WALL_TOP + BASE_BOT) / 2
        sym_stroke = "40"
        icon_grad = '''
        <linearGradient id="iconGrad" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stop-color="#7B3D26"/>
            <stop offset="100%" stop-color="#31150D"/>
        </linearGradient>
        '''
        if mode == 'infinite':
            sym_def = f'''
            <g id="sym" fill="none" stroke="url(#iconGrad)" stroke-width="{sym_stroke}" stroke-linecap="round" stroke-linejoin="round">
                <path d="M {f(BCX)},{f(BCY)} C {f(BCX+68)},{f(BCY-98)} {f(BCX+158)},{f(BCY-98)} {f(BCX+158)},{f(BCY)} C {f(BCX+158)},{f(BCY+98)} {f(BCX+68)},{f(BCY+98)} {f(BCX)},{f(BCY)} C {f(BCX-68)},{f(BCY-98)} {f(BCX-158)},{f(BCY-98)} {f(BCX-158)},{f(BCY)} C {f(BCX-158)},{f(BCY+98)} {f(BCX-68)},{f(BCY+98)} {f(BCX)},{f(BCY)} Z" />
            </g>'''
            badge_elements = icon_grad + sym_def + '<use href="#sym" />'
        elif mode == 'clock':
            sym_def = f'''
            <g id="sym" fill="none" stroke="url(#iconGrad)" stroke-width="{sym_stroke}" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="{f(BCX)}" cy="{f(BCY)}" r="135" />
                <polyline points="{f(BCX)},{f(BCY-68)} {f(BCX)},{f(BCY)} {f(BCX+43)},{f(BCY+43)}" />
            </g>'''
            badge_elements = icon_grad + sym_def + '<use href="#sym" />'
        elif mode == 'calendar':
            sym_def = f'''
            <g id="sym_stroke" fill="none" stroke="url(#iconGrad)" stroke-width="{sym_stroke}" stroke-linecap="round" stroke-linejoin="round">
                <rect x="{f(BCX-125)}" y="{f(BCY-125)}" width="250" height="250" rx="42" />
            </g>
            <g id="sym_fill" stroke="none" fill="url(#iconGrad)">
                <circle cx="{f(BCX-62)}" cy="{f(BCY-25)}" r="18" />
                <circle cx="{f(BCX)}" cy="{f(BCY-25)}" r="18" />
                <circle cx="{f(BCX+62)}" cy="{f(BCY-25)}" r="18" />
                <circle cx="{f(BCX-62)}" cy="{f(BCY+50)}" r="18" />
                <circle cx="{f(BCX)}" cy="{f(BCY+50)}" r="18" />
            </g>
            '''
            badge_elements = icon_grad + sym_def + '<use href="#sym_stroke" /><use href="#sym_fill" />'

    # ---- viewBox: closed-form bbox of body-rect + handle outer-arc, no sampling ----
    half_stroke = OUTLINE_WIDTH / 2
    x_min = WALL_L - half_stroke
    x_max = (HX + RO) + half_stroke
    y_min = WALL_TOP - half_stroke
    y_max = BASE_BOT + half_stroke

    PAD_RATIO = 0.06
    bw, bh = x_max - x_min, y_max - y_min
    side = max(bw, bh) * (1 + PAD_RATIO)
    ccx, ccy = (x_min + x_max) / 2, (y_min + y_max) / 2
    VIEWBOX = f"{f(ccx - side/2)} {f(ccy - side/2)} {f(side)} {f(side)}"

    svg = f'''<?xml version="1.0" encoding="utf-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="{VIEWBOX}" width="1024px" height="1024px">
  <defs>
    <linearGradient id="flatGrad" x1="{f(WALL_L)}" y1="0" x2="{f(HX + RO)}" y2="0" gradientUnits="userSpaceOnUse">
        {grad_stops}
    </linearGradient>
    <linearGradient id="bodyVerticalShadow" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#FFFFFF" stop-opacity="0.5"/>
      <stop offset="60%" stop-color="#FFFFFF" stop-opacity="0"/>
      <stop offset="100%" stop-color="#303A45" stop-opacity="{vertical_shadow_opacity}"/>
    </linearGradient>
  </defs>

  <!-- Outline Silhouette Layer -->
  <g stroke="{OUTLINE_COLOR}" stroke-width="{OUTLINE_WIDTH}" stroke-linejoin="round" stroke-linecap="round">
    <path d="{handle_d}" fill="{OUTLINE_COLOR}" />
    <path d="{cup_body_d}" fill="{OUTLINE_COLOR}" />
  </g>

  <!-- Inner Fill Layer -->
  <g>
    <path d="{handle_d}" fill="url(#flatGrad)" />
    <path d="{handle_d}" fill="url(#bodyVerticalShadow)" />
    <path d="{cup_body_d}" fill="url(#flatGrad)" />
    <path d="{cup_body_d}" fill="url(#bodyVerticalShadow)" />
  </g>

  <!-- Central Icon Layer -->
  {badge_elements}
</svg>
'''
    return svg

open('tray_inactive.svg', 'w').write(gen_tray_svg_final('inactive'))
open('tray_infinite.svg', 'w').write(gen_tray_svg_final('infinite'))
open('tray_clock.svg', 'w').write(gen_tray_svg_final('clock'))
open('tray_calendar.svg', 'w').write(gen_tray_svg_final('calendar'))
print("done")