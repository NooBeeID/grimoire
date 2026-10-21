import { post } from '../../../../../shared/services/http';
import { hasControlChars, runeLength } from '../../../../../shared/utils/text';
import {
  CreatedProjectSchema,
  DESCRIPTION_MAX,
  NAME_MAX,
  NAME_MIN,
  type CreatedProject,
  type CreateProjectInput,
} from '../types';

// Validasi klien hanya mencegah request yang pasti gagal; server tetap sumber kebenaran.
export function validateCreate(i: CreateProjectInput): Record<string, string> {
  const errors: Record<string, string> = {};
  const name = i.name.trim();
  const n = runeLength(name);
  if (n < NAME_MIN || n > NAME_MAX) errors.name = 'project.validation.nameLength';
  else if (hasControlChars(name)) errors.name = 'project.validation.nameControl';
  const d = i.description.trim();
  if (runeLength(d) > DESCRIPTION_MAX) errors.description = 'project.validation.descriptionLength';
  else if (hasControlChars(d, ['\n', '\r', '\t'])) errors.description = 'project.validation.descriptionControl';
  return errors;
}

export function createProject(i: CreateProjectInput): Promise<CreatedProject> {
  return post('/api/projects', { name: i.name.trim(), description: i.description.trim() }, { schema: CreatedProjectSchema });
}
