-- Revert to original (broken) schemas from 000016
UPDATE form_schemas SET
  schema_def = '{"sections":[{"id":"sales","name":"Sales/Receipts","fields":[{"id":"vatable_sales","line":"14A","label":"Vatable Sales","editable":true,"required":true},{"id":"vat_exempt_sales","line":"14B","label":"VAT-Exempt Sales","editable":true},{"id":"zero_rated_sales","line":"14C","label":"Zero-Rated Sales","editable":true}]},{"id":"purchases","name":"Purchases","fields":[{"id":"domestic_purchases_goods","line":"18A","label":"Domestic Purchases of Goods","editable":true},{"id":"domestic_purchases_services","line":"18B","label":"Domestic Purchases of Services","editable":true}]}]}',
  calculation_rules = '{"output_tax":"vatable_sales * 0.12","input_tax":"(domestic_purchases_goods + domestic_purchases_services) * 0.12","vat_payable":"output_tax - input_tax"}',
  version = version - 1
WHERE form_type = 'BIR_2550Q';

UPDATE form_schemas SET
  schema_def = '{"sections":[{"id":"sales","name":"Sales/Receipts","fields":[{"id":"vatable_sales","line":"14A","label":"Vatable Sales","editable":true,"required":true},{"id":"vat_exempt_sales","line":"14B","label":"VAT-Exempt Sales","editable":true},{"id":"zero_rated_sales","line":"14C","label":"Zero-Rated Sales","editable":true}]},{"id":"purchases","name":"Purchases","fields":[{"id":"domestic_purchases","line":"18A","label":"Domestic Purchases of Goods","editable":true},{"id":"domestic_services","line":"18B","label":"Domestic Purchases of Services","editable":true},{"id":"importation","line":"18C","label":"Importation of Goods","editable":true}]},{"id":"tax","name":"Tax Computation","fields":[{"id":"output_tax","line":"15","label":"Output Tax","editable":false},{"id":"input_tax","line":"19","label":"Input Tax","editable":false},{"id":"vat_payable","line":"20","label":"VAT Payable","editable":false}]}]}',
  calculation_rules = '{"output_tax":"vatable_sales * 0.12","input_tax":"(domestic_purchases + domestic_services + importation) * 0.12","vat_payable":"output_tax - input_tax"}',
  version = version - 1
WHERE form_type = 'BIR_2550M';
