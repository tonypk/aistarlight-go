-- Seed knowledge_chunks with essential Philippine BIR tax regulations

INSERT INTO knowledge_chunks (id, source, category, content, metadata) VALUES

-- VAT Basics
(uuid_generate_v4(), 'NIRC Section 106-108', 'vat',
'Philippine VAT Rate: The standard VAT rate is 12% on the gross selling price or gross value in money of goods, properties, or services. VAT-exempt transactions include sale of agricultural products in their original state, educational services, and medical/dental services. Zero-rated sales include exports and foreign currency denominated sales.',
'{"section": "106-108", "law": "NIRC"}'),

(uuid_generate_v4(), 'RR No. 16-2005', 'vat',
'VAT Computation: Output VAT = Vatable Sales × 12%. Input VAT = VAT paid on purchases of goods and services used in business. Net VAT Payable = Output VAT - Input VAT. If Input VAT exceeds Output VAT, the excess may be carried over to next quarter or applied for refund/TCC.',
'{"regulation": "RR 16-2005"}'),

(uuid_generate_v4(), 'NIRC Section 114', 'vat',
'VAT Filing Deadlines: BIR Form 2550M (Monthly VAT) is due on or before the 20th day of the following month. BIR Form 2550Q (Quarterly VAT) is due on or before the 25th day of the month following the close of the quarter. Late filing incurs 25% surcharge + 20% per annum interest.',
'{"section": "114", "law": "NIRC"}'),

(uuid_generate_v4(), 'RR No. 13-2018', 'vat',
'VAT-Exempt Threshold: Beginning January 1, 2023, businesses with gross annual sales not exceeding PHP 3,000,000 are VAT-exempt but may be subject to 3% percentage tax under TRAIN law. Businesses exceeding this threshold must register as VAT taxpayers.',
'{"regulation": "RR 13-2018", "law": "TRAIN"}'),

(uuid_generate_v4(), 'BIR Form 2550M Instructions', 'vat',
'BIR 2550M Structure: Part I - Background Information (TIN, company name, address). Part II - Sales/Receipts (Lines 1-5: vatable sales, government sales, zero-rated, exempt, total). Part III - Output Tax (Lines 6-6B). Part IV - Input Tax (Lines 7-11). Part V - Tax Credit/Payments. Total Amount Due on Line 16.',
'{"form": "BIR_2550M"}'),

-- Withholding Tax
(uuid_generate_v4(), 'RR No. 2-98 as amended', 'withholding',
'Expanded Withholding Tax (EWT): Creditable withholding tax rates vary by type of payment. Professional fees: 5% (individual) or 10% (corporate) if gross income exceeds PHP 3M. Rental of property: 5%. Services by contractors: 2%. Interest on bank deposits: 20% final tax.',
'{"regulation": "RR 2-98"}'),

(uuid_generate_v4(), 'BIR Form 0619-E Instructions', 'withholding',
'BIR 0619-E (Monthly Remittance of Creditable Income Tax Withheld - Expanded): Due on or before the 10th day of the following month for monthly filing, or the last day of the following month for quarterly filing. Must be accompanied by the alphabetical list of payees (BIR Form 2307).',
'{"form": "BIR_0619E"}'),

(uuid_generate_v4(), 'RR No. 11-2018', 'withholding',
'Compensation Tax (BIR 1601-C): Employers must withhold tax on compensation based on the graduated tax rate table. Monthly remittance via BIR 1601-C due on or before the 10th day of the following month. Annual reconciliation via BIR 1604-CF.',
'{"form": "BIR_1601C", "regulation": "RR 11-2018"}'),

(uuid_generate_v4(), 'BIR Form 2307', 'withholding',
'Certificate of Creditable Tax Withheld at Source (BIR 2307): Issued by the withholding agent to the payee. Must be attached to the income tax return. Contains: TIN of withholding agent and payee, nature of payment, amount of income, tax withheld. ATC (Alphanumeric Tax Code) identifies the type of payment.',
'{"form": "BIR_2307"}'),

-- Income Tax
(uuid_generate_v4(), 'NIRC Section 24 (TRAIN Law)', 'income_tax',
'Individual Income Tax Rates (TRAIN Law, effective 2023): Not over PHP 250,000: 0%. Over 250K to 400K: 15% of excess over 250K. Over 400K to 800K: 22,500 + 20% of excess over 400K. Over 800K to 2M: 102,500 + 25% of excess over 800K. Over 2M to 8M: 402,500 + 30% of excess over 2M. Over 8M: 2,202,500 + 35% of excess over 8M.',
'{"section": "24", "law": "TRAIN"}'),

(uuid_generate_v4(), 'NIRC Section 27', 'income_tax',
'Corporate Income Tax: Regular Corporate Income Tax (RCIT) rate is 25% for domestic corporations with net taxable income above PHP 5M and total assets above PHP 100M. 20% for corporations that do not meet both thresholds. Minimum Corporate Income Tax (MCIT) is 1% of gross income, applicable starting 4th year of operation.',
'{"section": "27", "law": "CREATE"}'),

(uuid_generate_v4(), 'BIR Form 1701 Instructions', 'income_tax',
'BIR 1701 (Annual ITR for Individuals): Filed by self-employed individuals, professionals, and those with mixed income. Due on or before April 15 of the following year. Includes schedules for gross sales/receipts, cost of sales, operating expenses, and other income. 8% flat tax option available for gross sales/receipts not exceeding PHP 3M.',
'{"form": "BIR_1701"}'),

(uuid_generate_v4(), 'BIR Form 1702 Instructions', 'income_tax',
'BIR 1702 (Annual ITR for Corporations): Filed by domestic and resident foreign corporations. Due on or before the 15th day of the 4th month following the close of the taxable year. Quarterly returns (BIR 1702Q) due within 60 days after each quarter. Includes MCIT computation.',
'{"form": "BIR_1702"}'),

-- Compliance
(uuid_generate_v4(), 'RR No. 7-2024', 'compliance',
'TIN Format: Philippine Tax Identification Number format is ###-###-###-### (12 digits with 3 dashes). Every person subject to internal revenue taxes must register and obtain a TIN. A person may have only one TIN. Penalties for non-registration or multiple TINs.',
'{"regulation": "RR 7-2024"}'),

(uuid_generate_v4(), 'NIRC Section 248-249', 'compliance',
'Penalties for Late Filing: Surcharge of 25% of the tax due for failure to file or late filing. Interest of 12% per annum (double the legal interest rate) on any unpaid amount from the date prescribed for payment until full payment. Compromise penalty may also apply based on BIR schedule.',
'{"section": "248-249", "law": "NIRC"}'),

(uuid_generate_v4(), 'NIRC Section 237', 'compliance',
'Invoice and Receipt Requirements: VAT-registered taxpayers must issue VAT invoices for sale of goods and VAT official receipts for sale of services. Must contain: TIN, business name, business address, date of transaction, quantity/description, selling price, VAT amount shown separately, and the word "VAT" on the document.',
'{"section": "237", "law": "NIRC"}'),

-- General / SAWT
(uuid_generate_v4(), 'RR No. 1-2014', 'general',
'SAWT (Summary Alphalist of Withholding Taxes): Required attachment to quarterly and annual income tax returns. Lists all tax certificates (BIR 2307) received during the period. Must include: TIN of withholding agent, income payments, tax withheld. Must be submitted as CSV file via eFPS or eBIRForms.',
'{"regulation": "RR 1-2014", "form": "SAWT"}'),

(uuid_generate_v4(), 'BIR Filing Calendar', 'compliance',
'Key BIR Filing Deadlines: Monthly VAT (2550M): 20th of following month. Quarterly VAT (2550Q): 25th of month after quarter. Monthly EWT (0619-E): 10th of following month. Compensation Tax (1601-C): 10th of following month. Annual ITR Individual (1701): April 15. Annual ITR Corporate (1702): 15th of 4th month after fiscal year.',
'{"type": "calendar"}'),

(uuid_generate_v4(), 'CREATE Law (RA 11534)', 'general',
'CREATE Law Key Provisions: Reduced corporate income tax to 25% (from 30%). 20% for small corporations. MCIT reduced to 1% (from 2%) for 2020-2023. Rationalized fiscal incentives with sunset provisions. Enhanced deductions for research and development. VAT exemption for certain imported goods.',
'{"law": "CREATE", "regulation": "RA 11534"}')

ON CONFLICT DO NOTHING;
