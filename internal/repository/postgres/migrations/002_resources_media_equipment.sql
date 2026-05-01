CREATE TABLE IF NOT EXISTS resource_images (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS equipment (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS resource_equipment (
    resource_id BIGINT NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    equipment_id BIGINT NOT NULL REFERENCES equipment(id) ON DELETE CASCADE,
    PRIMARY KEY (resource_id, equipment_id)
);

CREATE INDEX IF NOT EXISTS idx_resource_images_resource_id ON resource_images(resource_id);
CREATE INDEX IF NOT EXISTS idx_equipment_slug ON equipment(slug);
CREATE INDEX IF NOT EXISTS idx_resource_equipment_resource_id ON resource_equipment(resource_id);
CREATE INDEX IF NOT EXISTS idx_resource_equipment_equipment_id ON resource_equipment(equipment_id);
