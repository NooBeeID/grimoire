// Package contentcount adalah kontrak consumer-driven untuk hitungan isi per project.
// Consumer: T03 (listprojects, getproject, deleteproject), fail-soft.
// Provider (satu endpoint per module, kontrak sama):
//
//	constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES (T05): jumlah file knowledge
//	constants.BUS_PROJECT_APISPEC_COUNT_SPECS   (T06): jumlah API spec
//	constants.BUS_PROJECT_APIKEY_COUNT_KEYS     (T04): jumlah key AKTIF (belum dicabut)
//
// Provider WAJIB: bus.Expect[Request]; hanya menghitung baris dengan project_id di ProjectIDs (rule 6.10);
// id bukan uuid diabaikan; id tanpa baris boleh tidak ada di Counts (consumer membaca 0);
// len(ProjectIDs) > MaxProjectIDs -> error membungkus errs.ErrInvalid; satu query GROUP BY.
package contentcount

const MaxProjectIDs = 100

type Request struct{ ProjectIDs []string }

type Response struct{ Counts map[string]int }
