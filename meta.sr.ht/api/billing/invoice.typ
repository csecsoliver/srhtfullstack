#set page(
  paper: "a4",
  margin: (x: 2cm, y: 1.5cm),
)
{{ if .Sample }}
#set page(background: rotate(24deg,
  text(96pt, fill: rgb("FFCBC4"))[
    *SAMPLE*
  ]
))
{{ end }}
#set text(
  font: "Noto Sans",
  size: 12pt
)
#set table(
  fill: (x, y) =>
    if y == 0 {
      gray.lighten(40%)
    }
)

#grid(
  columns: (1fr, 1fr),
  align(left + top)[
    {{ template "letterhead" . }}
  ],
  align(left + top)[
    #v(18pt)
    = #underline[Invoice to]
    #v(18pt)
    #grid(
      columns: (1fr, 1fr),
      row-gutter: 10pt,
      align(left)[
        *Invoice №* \
        *Invoice date* \
        *User account* \
      ],
      align(left)[
        {{ .Invoice.InvoiceNo }} \
        {{.Invoice.Issued.Format "02 Jan 2006"}} \
        #"{{ .User.CanonicalName }}" \
      ],
      {{- with .Address }}
      grid.cell(
        colspan: 2
      )[#line(length: 100%)],
      align(left)[
        *Billing address*
      ],
      align(left)[
        {{- with .FullName }}
        #"{{ . | escape }}" \
        {{- end }}
        {{- with .BusinessName }}
        #"{{ . | escape }}" \
        {{- end }}
        {{- with .Address1 }}
        #"{{ . | escape }}" \
        {{- end }}
        {{- with .Address2 }}
        #"{{ . | escape }}" \
        {{- end }}
        {{- with .City }}
        #"{{ . | escape }}" \
        {{- end }}
        {{- with .Region }}
        #"{{ . | escape }}" \
        {{- end }}
        {{- with .Country }}
        #"{{ . | countryName }}" \
        {{- end }}
        {{- with .Vat }}
        #"{{ . | escape }}" \
        {{- end }}
      ]
      {{- end }}
    )
  ]
)

#v(18pt)

{{ $c := .Invoice.Currency }}
#table(
  stroke: none,
  columns: (1fr, auto, 80pt, 80pt),
  table.hline(stroke: 0.5pt),
  table.header(
    [*Product*], [*Unit price*], [*Quantity*], [*Price*],
  ),
  table.hline(stroke: 0.5pt),
  {{- range $p := .Products }}
  "{{$p.Product.Name}}",
  "{{$p.UnitPrice.Amount | formatPrice $c}} p.m.",
  "{{$p.Units}} month(s)",
  "{{$p.Total | formatPrice $c}}",
  table.hline(stroke: 0.5pt),
  {{- end }}
  {{- range $d := .Discounts }}
  [ {{$d.Reason}} ],
  [ n/a ],
  [ n/a ],
  "({{$d.Amount | formatPrice $c}})",
  table.hline(stroke: 0.5pt),
  {{- end }}
  {{- if .Invoice.TaxRate }}
  [], [], [ *Subtotal* ], "{{.Invoice.Subtotal | formatPrice $c}}",
  table.hline(start: 2, stroke: 0.5pt),
  [], [], [ VAT ({{.Invoice.TaxRate | formatPercent}}) ], "{{.Invoice.TaxCharged | formatPrice $c}}",
  table.hline(start: 2, stroke: 0.5pt),
  {{- end }}
  [], [], [ *Total* ], strong("{{.Invoice.Total | formatPrice $c }}"),
  table.hline(start: 2, stroke: 1.5pt),
)

{{ if .Invoice.ReverseVAT }}
VAT for this invoice is reverse charged to customer (_btw verlegd_).
{{ end }}

#v(18pt)

Payment was automatically drawn from your account on file on
{{.Invoice.Issued.Format "02 Jan 2006"}}.
