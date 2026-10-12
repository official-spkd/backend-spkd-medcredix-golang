CREATE TABLE regulations (
    id          VARCHAR(40)  PRIMARY KEY,
    code        VARCHAR(40)  NOT NULL UNIQUE,
    title       VARCHAR(255) NOT NULL,
    number      VARCHAR(60),
    authority   VARCHAR(120),
    year        SMALLINT,
    status      VARCHAR(20)  NOT NULL DEFAULT 'active'
);

CREATE TABLE regulation_versions (
    id            VARCHAR(40) PRIMARY KEY,
    regulation_id VARCHAR(40) NOT NULL REFERENCES regulations(id),
    version       VARCHAR(30) NOT NULL,
    issued_at     DATE,
    impact_level  VARCHAR(20)
);


CREATE TABLE hospitals (
    id                 VARCHAR(40)  PRIMARY KEY,
    name               VARCHAR(200) NOT NULL,
    code               VARCHAR(40)  NOT NULL UNIQUE,
    hospital_class     VARCHAR(5),               
    province           VARCHAR(80),
    city               VARCHAR(80),
    bed_licensed       INTEGER      NOT NULL DEFAULT 0,
    bpjs_facility_code VARCHAR(30),
    satusehat_org_id   VARCHAR(80),
    tier               VARCHAR(30),              
    onboarding         VARCHAR(30),              
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id            VARCHAR(40)  PRIMARY KEY,
    hospital_id   VARCHAR(40)  REFERENCES hospitals(id),  
    email         VARCHAR(190) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,                  
    name          VARCHAR(150) NOT NULL,
    title         VARCHAR(120),
    role          VARCHAR(30)  NOT NULL
        CHECK (role IN ('super_admin','admin_rs','hospital_mgmt','credential_officer',
                        'medical_committee','hr_sdm','auditor','claims_team',
                        'it_simrs','viewer')),
    active        BOOLEAN      NOT NULL DEFAULT TRUE,
    last_login    TIMESTAMPTZ
);

CREATE TABLE practitioners (
    id          VARCHAR(40)  PRIMARY KEY,
    hospital_id VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    full_name   VARCHAR(200) NOT NULL,
    profession  VARCHAR(80),
    specialty   VARCHAR(120),
    department  VARCHAR(120),
    str_number  VARCHAR(60),
    str_expiry  DATE,
    sip_number  VARCHAR(60),
    sip_expiry  DATE,
    fhir_id     VARCHAR(80)
);

CREATE TABLE practitioner_credentials (
    id              VARCHAR(40) PRIMARY KEY,
    practitioner_id VARCHAR(40) NOT NULL REFERENCES practitioners(id),
    hospital_id     VARCHAR(40) NOT NULL REFERENCES hospitals(id),
    type            VARCHAR(40) NOT NULL,        
    number          VARCHAR(80),
    issue_date      DATE,
    expiry_date     DATE,
    status          VARCHAR(30) NOT NULL DEFAULT 'diajukan'
        CHECK (status IN ('diajukan','verifikasi','tinjauan_komite','disetujui',
                          'aktif','segera_berakhir','kedaluwarsa','ditangguhkan')),
    version         INTEGER     NOT NULL DEFAULT 1,
    verified_by_id  VARCHAR(40) REFERENCES users(id),
    verified_at     TIMESTAMPTZ,
    approved_by_id  VARCHAR(40) REFERENCES users(id),
    approved_at     TIMESTAMPTZ
);

CREATE TABLE credential_documents (
    id             VARCHAR(40)  PRIMARY KEY,
    credential_id  VARCHAR(40)  NOT NULL REFERENCES practitioner_credentials(id),
    file_name      VARCHAR(255) NOT NULL,
    file_type      VARCHAR(40),
    size_kb        INTEGER,
    uploaded_by_id VARCHAR(40)  REFERENCES users(id),
    status         VARCHAR(30)
);

CREATE TABLE requirements (                       
    id                VARCHAR(40)  PRIMARY KEY,
    hospital_id       VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    code              VARCHAR(4)   NOT NULL CHECK (code IN ('P1','P2','P3','P4','P5','P6')),
    name              VARCHAR(200) NOT NULL,
    category          VARCHAR(60),
    regulatory_source VARCHAR(200),
    status            VARCHAR(30)  NOT NULL DEFAULT 'belum_dilengkapi'
        CHECK (status IN ('belum_dilengkapi','menunggu_verifikasi','valid')),
    doc_number        VARCHAR(80),
    issued_at         DATE,
    doc_expiry        DATE,
    file_name         VARCHAR(255),
    note              TEXT,
    UNIQUE (hospital_id, code)
);

CREATE TABLE audit_logs (                         
    id             VARCHAR(40)  PRIMARY KEY,
    hospital_id    VARCHAR(40)  REFERENCES hospitals(id),  
    actor_name     VARCHAR(150) NOT NULL,
    actor_role     VARCHAR(30)  NOT NULL,
    action         VARCHAR(60)  NOT NULL,
    entity_type    VARCHAR(60),
    entity_id      VARCHAR(40),
    description    TEXT,
    before_value   JSONB,
    after_value    JSONB,
    ip             VARCHAR(45),
    correlation_id VARCHAR(64),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE compliance_mappings (
    id            VARCHAR(40)  PRIMARY KEY,
    regulation_id VARCHAR(40)  NOT NULL REFERENCES regulations(id),
    hospital_id   VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    article       VARCHAR(80),
    module        VARCHAR(80),
    feature       VARCHAR(120),
    status        VARCHAR(30),
    owner         VARCHAR(120),
    reviewed_at   TIMESTAMPTZ
);





CREATE TABLE risks (
    id              VARCHAR(40)  PRIMARY KEY,
    hospital_id     VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    code            VARCHAR(30)  NOT NULL,
    category        VARCHAR(40)  NOT NULL,       
    level           VARCHAR(10)  NOT NULL CHECK (level IN ('kritis','tinggi','sedang','rendah')),
    title           VARCHAR(255) NOT NULL,
    description     TEXT,
    business_impact TEXT,
    recommendation  TEXT,
    suggested_owner VARCHAR(120),
    deadline        DATE,
    status          VARCHAR(20)  NOT NULL DEFAULT 'baru'
        CHECK (status IN ('baru','terbuka','ditinjau','dalam_pengerjaan',
                          'terkelola','teratasi','selesai','ditutup'))
);

CREATE TABLE early_warnings (
    id             VARCHAR(40)  PRIMARY KEY,
    hospital_id    VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    code           VARCHAR(30)  NOT NULL,
    title          VARCHAR(255) NOT NULL,
    severity       VARCHAR(10)  NOT NULL,
    threshold_rule VARCHAR(255),
    trigger_value  VARCHAR(80),
    status         VARCHAR(20)  NOT NULL DEFAULT 'baru'
        CHECK (status IN ('baru','diketahui','ditugaskan','teratasi')),
    assigned_to    VARCHAR(120),
    due_date       DATE
);

CREATE TABLE alert_rules (
    id             VARCHAR(40)  PRIMARY KEY,
    hospital_id    VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    name           VARCHAR(200) NOT NULL,
    domain         VARCHAR(60),
    metric         VARCHAR(80),
    rule_condition VARCHAR(120),
    severity       VARCHAR(10),
    channels       VARCHAR(80),                  
    active         BOOLEAN      NOT NULL DEFAULT TRUE
);

CREATE TABLE report_snapshots (
    id           VARCHAR(40)  PRIMARY KEY,
    hospital_id  VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    type         VARCHAR(40)  NOT NULL,          
    title        VARCHAR(255) NOT NULL,
    period       VARCHAR(20),
    version      VARCHAR(10)  NOT NULL,          
    audit_ref    VARCHAR(30),                    
    content_json JSONB        NOT NULL,
    created_by   VARCHAR(150),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE notifications (
    id          VARCHAR(40)  PRIMARY KEY,
    hospital_id VARCHAR(40)  REFERENCES hospitals(id),
    user_id     VARCHAR(40)  NOT NULL REFERENCES users(id),
    title       VARCHAR(255) NOT NULL,
    severity    VARCHAR(10),
    channel     VARCHAR(20),
    is_read     BOOLEAN      NOT NULL DEFAULT FALSE
);

CREATE TABLE actions (                           
    id              VARCHAR(40)  PRIMARY KEY,
    hospital_id     VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    code            VARCHAR(12)  NOT NULL,       
    title           VARCHAR(255) NOT NULL,
    description     TEXT,
    priority        VARCHAR(10)  NOT NULL DEFAULT 'sedang',
    status          VARCHAR(30)  NOT NULL DEFAULT 'baru'
        CHECK (status IN ('baru','ditugaskan','dalam_pengerjaan',
                          'menunggu_verifikasi','selesai','ditutup','ditolak','dibuka_kembali')),
    source_type     VARCHAR(30),                 
    source_ref      VARCHAR(60),
    owner           VARCHAR(120),
    due_date        DATE,
    expected_result TEXT,
    resolution_note TEXT,
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (hospital_id, code)
);

CREATE TABLE action_events (
    id         VARCHAR(40)  PRIMARY KEY,
    action_id  VARCHAR(40)  NOT NULL REFERENCES actions(id),
    type       VARCHAR(30)  NOT NULL,            
    from_value VARCHAR(80),
    to_value   VARCHAR(80),
    by_name    VARCHAR(150),
    note       TEXT,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE claims (
    id            VARCHAR(40)   PRIMARY KEY,
    hospital_id   VARCHAR(40)   NOT NULL REFERENCES hospitals(id),
    claim_number  VARCHAR(60)   NOT NULL,
    sep_number    VARCHAR(60),
    inacbg_code   VARCHAR(20),
    care_type     VARCHAR(20),                   
    billed_amount NUMERIC(16,2),
    status        VARCHAR(30),
    flags_json    JSONB,                         
    UNIQUE (hospital_id, claim_number)
);

CREATE TABLE claim_anomalies (
    id          VARCHAR(40)  PRIMARY KEY,
    claim_id    VARCHAR(40)  NOT NULL REFERENCES claims(id),
    hospital_id VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    category    VARCHAR(40),
    severity    VARCHAR(10),
    title       VARCHAR(255) NOT NULL,
    status      VARCHAR(30)  NOT NULL DEFAULT 'baru'
        CHECK (status IN ('baru','ditinjau','diklarifikasi','terkonfirmasi_bermasalah')),
    reviewed_by VARCHAR(150)
);

CREATE TABLE clinical_pathways (
    id                  VARCHAR(40) PRIMARY KEY,
    hospital_id         VARCHAR(40) NOT NULL REFERENCES hospitals(id),
    diagnosis           VARCHAR(200) NOT NULL,
    icd10               VARCHAR(10),
    standard_los        NUMERIC(5,1),
    avg_adherence       NUMERIC(5,2),
    documented_deviation NUMERIC(5,2)
);

CREATE TABLE pathway_episodes (
    id             VARCHAR(40) PRIMARY KEY,
    pathway_id     VARCHAR(40) NOT NULL REFERENCES clinical_pathways(id),
    episode_number VARCHAR(40) NOT NULL,
    adherence      NUMERIC(5,2),
    los_actual     NUMERIC(5,1),
    documented     BOOLEAN
);





CREATE TABLE data_quality_issues (
    id          VARCHAR(40)  PRIMARY KEY,
    hospital_id VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    dimension   VARCHAR(60),
    title       VARCHAR(255) NOT NULL,
    severity    VARCHAR(10),
    anomaly     BOOLEAN      NOT NULL DEFAULT FALSE,   
    status      VARCHAR(20)  NOT NULL DEFAULT 'terbuka'
        CHECK (status IN ('terbuka','diperbaiki'))
);

CREATE TABLE trust_factors (                    
    id          VARCHAR(40)  PRIMARY KEY,
    hospital_id VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    factor      VARCHAR(60)  NOT NULL,
    score       NUMERIC(5,2) NOT NULL,
    weight      NUMERIC(5,2) NOT NULL
);

CREATE TABLE score_snapshots (
    id              VARCHAR(40)  PRIMARY KEY,
    hospital_id     VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    period          VARCHAR(20)  NOT NULL,
    health_score    NUMERIC(5,2),
    bdcs            NUMERIC(5,2),
    trust           NUMERIC(5,2),
    ccs             NUMERIC(5,2),
    cpcs            NUMERIC(5,2),
    ur_score        NUMERIC(5,2),
    safety_score    NUMERIC(5,2),
    formula_version VARCHAR(40)  NOT NULL DEFAULT 'medcredix-scorer/1.0',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE evidence_items (
    id                VARCHAR(40)  PRIMARY KEY,
    hospital_id       VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    ref_type          VARCHAR(30)  NOT NULL,     
    ref_id            VARCHAR(40),
    metric            VARCHAR(120),
    metric_value      VARCHAR(80),
    source_system     VARCHAR(60),
    fhir_resource     VARCHAR(60),
    record_count      INTEGER,
    validation_status VARCHAR(20)
);

CREATE TABLE digital_indicators (
    id            VARCHAR(40)  PRIMARY KEY,
    hospital_id   VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    dimension     VARCHAR(60),
    indicator     VARCHAR(160) NOT NULL,
    metric_value  NUMERIC(10,2),
    threshold     NUMERIC(10,2),
    weight        NUMERIC(5,2),
    direction     VARCHAR(10),                   
    fhir_resource VARCHAR(60)
);

CREATE TABLE complaint_stats (
    id           VARCHAR(40) PRIMARY KEY,
    hospital_id  VARCHAR(40) NOT NULL REFERENCES hospitals(id),
    month        VARCHAR(7)  NOT NULL,           
    total        INTEGER,
    satisfaction NUMERIC(5,2),
    peer_avg     NUMERIC(5,2),
    UNIQUE (hospital_id, month)
);

CREATE TABLE data_sources (
    id           VARCHAR(40)  PRIMARY KEY,
    hospital_id  VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    name         VARCHAR(120) NOT NULL,
    type         VARCHAR(30),                    
    status       VARCHAR(20),
    auth_method  VARCHAR(30),                    
    success_rate NUMERIC(5,2),
    coverage_pct NUMERIC(5,2),
    last_sync_at TIMESTAMPTZ
);

CREATE TABLE integration_jobs (
    id             VARCHAR(40) PRIMARY KEY,
    data_source_id VARCHAR(40) NOT NULL REFERENCES data_sources(id),
    job_type       VARCHAR(40),
    status         VARCHAR(20),
    records_pulled INTEGER,
    duration_ms    INTEGER
);

CREATE TABLE facilities (
    id           VARCHAR(40)  PRIMARY KEY,
    hospital_id  VARCHAR(40)  NOT NULL REFERENCES hospitals(id),
    name         VARCHAR(160) NOT NULL,
    category     VARCHAR(60),
    availability BOOLEAN,
    functional   BOOLEAN,
    calibration  VARCHAR(30)
);

CREATE TABLE bed_stats (
    id          VARCHAR(40) PRIMARY KEY,
    hospital_id VARCHAR(40) NOT NULL REFERENCES hospitals(id),
    ward        VARCHAR(80),
    bed_class   VARCHAR(20),
    total       INTEGER,
    active      INTEGER
);

CREATE TABLE specialty_standards (
    id          VARCHAR(40) PRIMARY KEY,
    hospital_id VARCHAR(40) NOT NULL REFERENCES hospitals(id),
    rs_class    VARCHAR(5),
    specialty   VARCHAR(120),
    required    INTEGER
);

CREATE TABLE utilization_stats (
    id          VARCHAR(40) PRIMARY KEY,
    hospital_id VARCHAR(40) NOT NULL REFERENCES hospitals(id),
    month       VARCHAR(7)  NOT NULL,
    alos        NUMERIC(5,2),
    readmission NUMERIC(5,2),
    bor         NUMERIC(5,2),
    UNIQUE (hospital_id, month)
);




CREATE INDEX idx_users_hospital        ON users(hospital_id);
CREATE INDEX idx_practitioners_hosp    ON practitioners(hospital_id);
CREATE INDEX idx_credentials_hosp      ON practitioner_credentials(hospital_id, status);
CREATE INDEX idx_credentials_expiry    ON practitioner_credentials(expiry_date);
CREATE INDEX idx_credentials_pract     ON practitioner_credentials(practitioner_id);
CREATE INDEX idx_audit_hosp_time       ON audit_logs(hospital_id, created_at DESC);
CREATE INDEX idx_audit_entity          ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_risks_hosp            ON risks(hospital_id, status);
CREATE INDEX idx_warnings_hosp         ON early_warnings(hospital_id, status);
CREATE INDEX idx_actions_hosp          ON actions(hospital_id, status);
CREATE INDEX idx_claims_hosp           ON claims(hospital_id);
CREATE INDEX idx_anomalies_claim       ON claim_anomalies(claim_id);
CREATE INDEX idx_dqi_hosp              ON data_quality_issues(hospital_id, status);
CREATE INDEX idx_evidence_ref          ON evidence_items(hospital_id, ref_type, ref_id);
CREATE INDEX idx_score_snap_hosp      ON score_snapshots(hospital_id, period);
CREATE INDEX idx_notif_user            ON notifications(user_id, is_read);
