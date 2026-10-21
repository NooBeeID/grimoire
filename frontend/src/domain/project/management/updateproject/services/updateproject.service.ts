import { patch } from '../../../../../shared/services/http';
import { hasControlChars, runeLength } from '../../../../../shared/utils/text';
import {
  DESCRIPTION_MAX,
  NAME_MAX,
  NAME_MIN,
  UpdatedProjectSchema,
  type EditableProject,
  type ProjectPatch,
  type UpdatedProject,
} from '../types';

// Bandingkan setelah trim; hanya field yang berubah.
export function diffPatch(current: EditableProject, form: { name: string; description: string }): ProjectPatch {
  const out: ProjectPatch = {};
  const name = form.name.trim();
  const description = form.description.trim();
  if (name !== current.name.trim()) out.name = name;
  if (description !== current.description.trim()) out.description = description;
  return out;
}

export function validatePatch(p: ProjectPatch): Record<string, string> {
  const errors: Record<string, string> = {};
  if (p.name !== undefined) {
    const n = runeLength(p.name);
    if (n < NAME_MIN || n > NAME_MAX) errors.name = 'project.validation.nameLength';
    else if (hasControlChars(p.name)) errors.name = 'project.validation.nameControl';
  }
  if (p.description !== undefined) {
    if (runeLength(p.description) > DESCRIPTION_MAX) errors.description = 'project.validation.descriptionLength';
    else if (hasControlChars(p.description, ['\n', '\r', '\t'])) errors.description = 'project.validation.descriptionControl';
  }
  return errors;
}

export function updateProject(id: string, p: ProjectPatch): Promise<UpdatedProject> {
  return patch('/api/projects/' + encodeURIComponent(id), p, { schema: UpdatedProjectSchema });
}
