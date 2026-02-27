-- Seed knowledge_chunks with Singapore IRAS tax regulations (jurisdiction = 'SG')

INSERT INTO knowledge_chunks (id, source, category, content, metadata, jurisdiction) VALUES

-- GST Basics
(uuid_generate_v4(), 'GST Act Section 8', 'gst',
'Singapore GST Rate: The standard GST rate is 9% (effective 1 Jan 2024, increased from 8% in 2023 and 7% previously). GST applies to the supply of goods and services in Singapore by a taxable person in the course of business. Exempt supplies include sale/lease of residential property, financial services, and digital payment tokens.',
'{"section": "8", "law": "GST Act"}', 'SG'),

(uuid_generate_v4(), 'IRAS e-Tax Guide: GST General Guide', 'gst',
'GST Registration: Compulsory registration is required when taxable turnover exceeds S$1 million in the past 12 months, or is expected to exceed S$1 million in the next 12 months. Voluntary registration is possible with IRAS approval. Once registered, must remain for at least 2 years.',
'{"source": "IRAS e-Tax Guide"}', 'SG'),

(uuid_generate_v4(), 'GST Act Section 19', 'gst',
'GST Return (GST F5): GST-registered businesses must file GST F5 returns quarterly (or monthly with IRAS approval). Filing deadline: 1 month after the end of the accounting period. Returns must report: Standard-rated supplies (Box 1), Zero-rated supplies (Box 2), Exempt supplies (Box 3), Total supplies (Box 4), Taxable purchases (Box 5), Output tax (Box 6), Input tax (Box 7), Net GST (Box 8).',
'{"section": "19", "law": "GST Act", "form": "IRAS_GST_F5"}', 'SG'),

(uuid_generate_v4(), 'GST Act Section 20', 'gst',
'Input Tax Claims: A GST-registered person may claim input tax on goods/services acquired for business purposes, subject to conditions: (1) goods/services used for making taxable supplies, (2) supported by valid tax invoice, (3) claimed within 5 years. Blocked input tax: club subscriptions, medical expenses (unless mandatory), motor car expenses, family benefits.',
'{"section": "20", "law": "GST Act"}', 'SG'),

(uuid_generate_v4(), 'IRAS e-Tax Guide: GST Zero-Rating', 'gst',
'Zero-Rated Supplies: Exports of goods and international services are zero-rated (0% GST). Conditions: goods must be exported within 60 days of supply, export evidence required (bill of lading, airway bill). International services include services supplied to overseas persons not in Singapore.',
'{"source": "IRAS e-Tax Guide"}', 'SG'),

-- Corporate Income Tax
(uuid_generate_v4(), 'Income Tax Act Section 10', 'income_tax',
'Singapore Corporate Tax Rate: Flat 17% on chargeable income. Partial tax exemption for companies: 75% exemption on first S$10,000 of chargeable income, 50% exemption on next S$190,000. Effective rate can be as low as 4.25% on first S$200,000. Start-up companies (first 3 years): additional 75% exemption on first S$100,000.',
'{"section": "10", "law": "Income Tax Act"}', 'SG'),

(uuid_generate_v4(), 'IRAS: Filing Corporate Tax', 'income_tax',
'Form C / Form C-S Filing: Companies must file corporate income tax returns annually. Form C-S (simplified) for companies with: revenue ≤ S$5 million, only Singapore-sourced income, no capital allowance claims, no group relief claims. Form C (full) for all other companies. Filing deadline: 30 November each year. E-filing via mytax.iras.gov.sg.',
'{"form": "IRAS_FORM_C"}', 'SG'),

(uuid_generate_v4(), 'Income Tax Act Section 43', 'income_tax',
'Estimated Chargeable Income (ECI): Companies must file ECI within 3 months of their financial year-end. ECI is the estimate of chargeable income for the Year of Assessment. Companies with revenue ≤ S$5 million AND ECI is nil may be waived from ECI filing. Late filing penalty: S$200 per month, up to S$1,000.',
'{"section": "43", "law": "Income Tax Act", "form": "IRAS_ECI"}', 'SG'),

(uuid_generate_v4(), 'Income Tax Act Section 14', 'income_tax',
'Tax Deductible Expenses (Corporate): Expenses are deductible if: (1) incurred in the production of income, (2) not capital in nature, (3) not specifically prohibited. Common deductions: staff costs, rental, utilities, professional fees, depreciation via capital allowances. Non-deductible: fines, penalties, donations (separate relief), private expenses, capital expenditure.',
'{"section": "14", "law": "Income Tax Act"}', 'SG'),

(uuid_generate_v4(), 'Income Tax Act Section 19A', 'income_tax',
'Capital Allowances: Replaces depreciation for tax purposes. Rates: machinery/equipment (33.3% over 3 years or 100% one-year write-off for items ≤ S$5,000), motor vehicles (20% over 5 years, capped at S$35,000 per vehicle), renovation (33.3% over 3 years, capped at S$300,000 per 3-year period).',
'{"section": "19A", "law": "Income Tax Act"}', 'SG'),

-- Individual Income Tax
(uuid_generate_v4(), 'Income Tax Act Section 2', 'income_tax',
'Singapore Individual Tax: Residents taxed on progressive rates from 0% to 22% (24% for income above S$1M from YA 2024). First S$20,000: 0%. S$20,001-S$30,000: 2%. S$30,001-S$40,000: 3.5%. S$40,001-S$80,000: 7%. S$80,001-S$120,000: 11.5%. S$120,001-S$160,000: 15%. S$160,001-S$200,000: 18%. S$200,001-S$240,000: 19%. S$240,001-S$280,000: 19.5%. S$280,001-S$320,000: 20%. Above S$320,000: 22%.',
'{"section": "2", "law": "Income Tax Act"}', 'SG'),

(uuid_generate_v4(), 'IRAS: Filing Form B', 'income_tax',
'Form B Filing: For individuals with self-employment or business income. Filing deadline: 18 April (paper) or 18 April (e-Filing). Includes all sources of income: employment, trade/business, rental, interest, dividends. Tax reliefs: earned income relief, CPF relief, life insurance, course fees, parent/spouse/child relief. Year of Assessment = Calendar year income is earned.',
'{"form": "IRAS_FORM_B"}', 'SG'),

(uuid_generate_v4(), 'Income Tax Act Section 39', 'income_tax',
'Personal Tax Reliefs (Singapore): Earned income relief (up to S$1,000 if below 55, S$6,000 if 55-59, S$8,000 if 60+). CPF/Provident Fund relief (limited to ordinary wages). Life insurance relief (up to S$5,000). Course fees relief (up to S$5,500). Parent relief (S$9,000 per dependant). Spouse relief (S$2,000). Child relief (S$4,000 per qualifying child). Working mother child relief (15-25% of earned income per child).',
'{"section": "39", "law": "Income Tax Act"}', 'SG'),

-- Withholding Tax (S45)
(uuid_generate_v4(), 'Income Tax Act Section 45', 'withholding',
'Section 45 Withholding Tax: Any person paying specified income to non-residents must withhold tax and remit to IRAS. Filing deadline: by the 15th of the 2nd month from the date of payment. Applicable to: interest (15%), royalties (10%), technical/management fees (17%), director fees (22%), rental of moveable property (15%), SRS withdrawal (22%).',
'{"section": "45", "law": "Income Tax Act", "form": "IRAS_S45"}', 'SG'),

(uuid_generate_v4(), 'Income Tax Act Section 45A', 'withholding',
'S45 WHT Rates by Income Type: INT — Interest: 15%. ROY — Royalties / Intellectual Property: 10%. TECH — Technical / Management Fees: 17% (prevailing corporate rate). DIR — Director Fees (non-resident directors): 22%. RENT — Rental of Moveable Property: 15%. SFC — SRS Withdrawal by Non-Resident: 22%. Rates may be reduced under Double Taxation Agreements (DTAs).',
'{"section": "45A", "law": "Income Tax Act"}', 'SG'),

(uuid_generate_v4(), 'IRAS: DTA Network', 'withholding',
'Double Taxation Agreements (DTAs): Singapore has 90+ comprehensive DTAs. DTA benefits may reduce S45 WHT rates. To enjoy DTA rates, non-resident must provide Certificate of Residence from their home country tax authority. Common DTA rates: interest 10%, royalties 5-10%, technical fees 0%. Apply via S45 filing with DTA claim.',
'{"source": "IRAS DTA Guide"}', 'SG'),

-- CPF (Central Provident Fund)
(uuid_generate_v4(), 'CPF Act Section 7', 'payroll',
'CPF Contribution Rates (2024): For employees aged ≤55: Employee 20%, Employer 17% (total 37%). Aged 55-60: Employee 15%, Employer 15%. Aged 60-65: Employee 9.5%, Employer 11.5%. Aged 65-70: Employee 7%, Employer 9%. Above 70: Employee 5%, Employer 7.5%. Ordinary wage ceiling: S$6,800/month. Additional wages ceiling: S$102,000 - Total ordinary wages.',
'{"section": "7", "law": "CPF Act"}', 'SG'),

(uuid_generate_v4(), 'CPF Act', 'payroll',
'CPF Allocation Rates (Age ≤55): Ordinary Account (OA): 23%. Special Account (SA): 6%. Medisave Account (MA): 8%. CPF is mandatory for all Singapore Citizens and Permanent Residents employed in Singapore. Employer must pay CPF by 14th of the following month. Late payment incurs interest at 18% per annum.',
'{"law": "CPF Act"}', 'SG'),

(uuid_generate_v4(), 'IRAS: IR8A Filing', 'payroll',
'IR8A Return of Employee Remuneration: Employers must submit IR8A for all employees by 1 March. Reports: gross salary, bonus, director fees, allowances, benefits-in-kind, stock options, employer CPF contributions. Auto-Inclusion Scheme (AIS): employers with ≥5 employees must submit electronically. Penalties: failure to file — fine up to S$5,000 and/or imprisonment up to 6 months.',
'{"form": "IRAS_IR8A"}', 'SG'),

-- Compliance & Penalties
(uuid_generate_v4(), 'Income Tax Act Section 94', 'compliance',
'IRAS Penalties for Late Filing: Corporate tax (Form C/C-S): S$200 per month (max estimated NOA + penalties). Individual tax (Form B): S$200 per month (estimated NOA issued). GST F5: 5% late payment penalty + additional 2% per month (max 50%). ECI: S$200 per month (max S$1,000). S45 WHT: 5% penalty on tax amount + interest at prevailing rate.',
'{"section": "94", "law": "Income Tax Act"}', 'SG'),

(uuid_generate_v4(), 'IRAS: Compliance Requirements', 'compliance',
'Record-Keeping Requirements: Companies must retain business records for at least 5 years. GST-registered businesses: 5 years from the prescribed accounting period. Records include: invoices, receipts, vouchers, bank statements, contracts, accounting records. Failure to maintain records: fine up to S$5,000.',
'{"source": "IRAS Compliance Guide"}', 'SG'),

(uuid_generate_v4(), 'IRAS: mytax Portal', 'compliance',
'IRAS Filing Channels: mytax Portal (mytax.iras.gov.sg) is the primary e-Filing platform. Supports: GST F5, Form C/C-S, Form B, ECI, S45, IR8A (via AIS). CorpPass login required for business filing. SingPass login for individual filing. Most forms can only be filed electronically (paper filing restricted).',
'{"source": "IRAS"}', 'SG'),

-- General Tax Administration
(uuid_generate_v4(), 'Income Tax Act', 'general',
'Year of Assessment (YA): Singapore uses preceding year basis. YA 2025 = income earned in calendar year 2024. Companies: YA based on financial year ending in the preceding year (e.g., FY ending 31 Mar 2024 → YA 2025). Tax residence: company is tax resident if control and management exercised in Singapore.',
'{"law": "Income Tax Act"}', 'SG'),

(uuid_generate_v4(), 'Income Tax Act Section 13', 'general',
'Tax Exempt Income: Dividends from Singapore companies (one-tier system — tax paid at corporate level). Capital gains (no capital gains tax in Singapore). Foreign-sourced income for individuals (unless remitted). Companies: foreign-sourced income exempt if: (1) income taxed in foreign jurisdiction, (2) headline rate ≥ 15%, (3) income remitted to Singapore.',
'{"section": "13", "law": "Income Tax Act"}', 'SG'),

(uuid_generate_v4(), 'IRAS: Tax Incentives', 'general',
'Singapore Tax Incentives: Productivity & Innovation Credit (PIC) — expired but legacy claims possible. Research & Development (R&D) tax deduction: 250% on qualifying expenditure (extended). Pioneer incentive: reduced tax rate of 5-10% for qualifying activities. Development & Expansion Incentive (DEI): reduced rate on incremental income. IP Development Incentive: 5-10% on qualifying IP income.',
'{"source": "IRAS Tax Incentives Guide"}', 'SG');
