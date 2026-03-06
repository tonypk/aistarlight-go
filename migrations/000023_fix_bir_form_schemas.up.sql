-- ============================================================
-- Fix BIR 2550M and 2550Q form schemas
--
-- The original schemas in 000016 used field IDs that don't match
-- the tax engine output keys (e.g. "domestic_purchases_goods"
-- instead of "line_7_input_vat_goods"). This migration updates
-- both schemas to use the complete, correct field IDs that align
-- with the Go tax engine's CalculateBIR2550M output.
-- ============================================================

UPDATE form_schemas SET
  schema_def = '{
    "sections": [
      {
        "id": "part2_sales",
        "name": "Sales/Receipts",
        "fields": [
          {"id": "line_1_vatable_sales", "line": "14A", "label": "Vatable Sales / Receipts", "editable": true, "required": false},
          {"id": "line_2_sales_to_government", "line": "14B", "label": "Sales to Government (5% Final Withholding VAT)", "editable": true, "required": false},
          {"id": "line_3_zero_rated_sales", "line": "14C", "label": "Zero-Rated Sales", "editable": true, "required": false},
          {"id": "line_4_exempt_sales", "line": "14D", "label": "VAT-Exempt Sales", "editable": true, "required": false},
          {"id": "line_5_total_sales", "line": "15", "label": "Total Sales / Receipts (Sum of Lines 14A to 14D)", "editable": false}
        ]
      },
      {
        "id": "part3_output_tax",
        "name": "Output Tax",
        "fields": [
          {"id": "line_6_output_vat", "line": "15A", "label": "Output VAT (12%)", "editable": false},
          {"id": "line_6a_output_vat_government", "line": "15B", "label": "Output VAT on Government Sales (5%)", "editable": false},
          {"id": "line_6b_total_output_vat", "line": "16", "label": "Total Output Tax", "editable": false}
        ]
      },
      {
        "id": "part4_input_tax",
        "name": "Input Tax",
        "fields": [
          {"id": "line_7_input_vat_goods", "line": "17", "label": "Input VAT — Domestic Purchases of Goods", "editable": true, "required": false},
          {"id": "line_8_input_vat_capital", "line": "18", "label": "Input VAT — Capital Goods", "editable": true, "required": false},
          {"id": "line_9_input_vat_services", "line": "19", "label": "Input VAT — Domestic Purchases of Services", "editable": true, "required": false},
          {"id": "line_10_input_vat_imports", "line": "20", "label": "Input VAT — Importation of Goods", "editable": true, "required": false},
          {"id": "line_11_total_input_vat", "line": "21", "label": "Total Input Tax (Sum of Lines 17 to 20)", "editable": false}
        ]
      },
      {
        "id": "part5_tax_due",
        "name": "Tax Due",
        "fields": [
          {"id": "line_12_vat_payable", "line": "22", "label": "VAT Payable (Output Tax - Input Tax)", "editable": false},
          {"id": "line_13_less_tax_credits", "line": "23", "label": "Less: Tax Credits / Payments", "editable": true, "required": false},
          {"id": "line_14_net_vat_payable", "line": "24", "label": "Net VAT Payable", "editable": false},
          {"id": "line_15_add_penalties", "line": "25", "label": "Add: Penalties", "editable": true, "required": false},
          {"id": "line_16_total_amount_due", "line": "26", "label": "TOTAL AMOUNT DUE", "editable": false}
        ]
      }
    ]
  }',
  calculation_rules = '{
    "line_5_total_sales": "line_1_vatable_sales + line_2_sales_to_government + line_3_zero_rated_sales + line_4_exempt_sales",
    "line_6_output_vat": "line_1_vatable_sales * 0.12",
    "line_6a_output_vat_government": "line_2_sales_to_government * 0.05",
    "line_6b_total_output_vat": "line_6_output_vat + line_6a_output_vat_government",
    "line_11_total_input_vat": "line_7_input_vat_goods + line_8_input_vat_capital + line_9_input_vat_services + line_10_input_vat_imports",
    "line_12_vat_payable": "line_6b_total_output_vat - line_11_total_input_vat",
    "line_14_net_vat_payable": "max(line_12_vat_payable - line_13_less_tax_credits, 0)",
    "line_16_total_amount_due": "line_14_net_vat_payable + line_15_add_penalties"
  }',
  version = version + 1
WHERE form_type = 'BIR_2550Q';

UPDATE form_schemas SET
  schema_def = '{
    "sections": [
      {
        "id": "part2_sales",
        "name": "Sales/Receipts",
        "fields": [
          {"id": "line_1_vatable_sales", "line": "1", "label": "Vatable Sales / Receipts", "editable": true, "required": false},
          {"id": "line_2_sales_to_government", "line": "2", "label": "Sales to Government (5% Final Withholding VAT)", "editable": true, "required": false},
          {"id": "line_3_zero_rated_sales", "line": "3", "label": "Zero-Rated Sales", "editable": true, "required": false},
          {"id": "line_4_exempt_sales", "line": "4", "label": "VAT-Exempt Sales", "editable": true, "required": false},
          {"id": "line_5_total_sales", "line": "5", "label": "Total Sales / Receipts (Sum of Lines 1 to 4)", "editable": false}
        ]
      },
      {
        "id": "part3_output_tax",
        "name": "Output Tax",
        "fields": [
          {"id": "line_6_output_vat", "line": "6", "label": "Output VAT (12%)", "editable": false},
          {"id": "line_6a_output_vat_government", "line": "6A", "label": "Output VAT on Government Sales (5%)", "editable": false},
          {"id": "line_6b_total_output_vat", "line": "6B", "label": "Total Output Tax", "editable": false}
        ]
      },
      {
        "id": "part4_input_tax",
        "name": "Input Tax",
        "fields": [
          {"id": "line_7_input_vat_goods", "line": "7", "label": "Input VAT — Domestic Purchases of Goods", "editable": true, "required": false},
          {"id": "line_8_input_vat_capital", "line": "8", "label": "Input VAT — Capital Goods", "editable": true, "required": false},
          {"id": "line_9_input_vat_services", "line": "9", "label": "Input VAT — Domestic Purchases of Services", "editable": true, "required": false},
          {"id": "line_10_input_vat_imports", "line": "10", "label": "Input VAT — Importation of Goods", "editable": true, "required": false},
          {"id": "line_11_total_input_vat", "line": "11", "label": "Total Input Tax (Sum of Lines 7 to 10)", "editable": false}
        ]
      },
      {
        "id": "part5_tax_due",
        "name": "Tax Due",
        "fields": [
          {"id": "line_12_vat_payable", "line": "12", "label": "VAT Payable (Output Tax - Input Tax)", "editable": false},
          {"id": "line_13_less_tax_credits", "line": "13", "label": "Less: Tax Credits / Payments", "editable": true, "required": false},
          {"id": "line_14_net_vat_payable", "line": "14", "label": "Net VAT Payable", "editable": false},
          {"id": "line_15_add_penalties", "line": "15", "label": "Add: Penalties", "editable": true, "required": false},
          {"id": "line_16_total_amount_due", "line": "16", "label": "TOTAL AMOUNT DUE", "editable": false}
        ]
      }
    ]
  }',
  calculation_rules = '{
    "line_5_total_sales": "line_1_vatable_sales + line_2_sales_to_government + line_3_zero_rated_sales + line_4_exempt_sales",
    "line_6_output_vat": "line_1_vatable_sales * 0.12",
    "line_6a_output_vat_government": "line_2_sales_to_government * 0.05",
    "line_6b_total_output_vat": "line_6_output_vat + line_6a_output_vat_government",
    "line_11_total_input_vat": "line_7_input_vat_goods + line_8_input_vat_capital + line_9_input_vat_services + line_10_input_vat_imports",
    "line_12_vat_payable": "line_6b_total_output_vat - line_11_total_input_vat",
    "line_14_net_vat_payable": "max(line_12_vat_payable - line_13_less_tax_credits, 0)",
    "line_16_total_amount_due": "line_14_net_vat_payable + line_15_add_penalties"
  }',
  version = version + 1
WHERE form_type = 'BIR_2550M';
