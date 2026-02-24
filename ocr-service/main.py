"""PaddleOCR microservice — extracts text from receipt images."""

import logging
import tempfile
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI, File, HTTPException, UploadFile
from paddleocr import PaddleOCR

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

_ocr_engine: PaddleOCR | None = None

ALLOWED_EXTENSIONS = {".jpg", ".jpeg", ".png", ".bmp", ".tiff", ".tif", ".webp"}
MAX_FILE_SIZE = 20 * 1024 * 1024  # 20 MB


def get_ocr() -> PaddleOCR:
    global _ocr_engine
    if _ocr_engine is None:
        logger.info("Initializing PaddleOCR engine (PP-OCRv5, CPU, en)...")
        _ocr_engine = PaddleOCR(
            ocr_version="PP-OCRv5",
            lang="en",
            device="cpu",
        )
        logger.info("PaddleOCR engine ready.")
    return _ocr_engine


@asynccontextmanager
async def lifespan(app: FastAPI):
    get_ocr()
    yield


app = FastAPI(title="OCR Service", version="1.0.0", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok", "engine": "PP-OCRv5"}


@app.post("/ocr")
async def ocr_image(file: UploadFile = File(...)):
    suffix = Path(file.filename or "unknown.jpg").suffix.lower()
    if suffix not in ALLOWED_EXTENSIONS:
        raise HTTPException(
            status_code=400,
            detail=f"Unsupported file type: {suffix}. Allowed: {', '.join(ALLOWED_EXTENSIONS)}",
        )

    content = await file.read()
    if len(content) > MAX_FILE_SIZE:
        raise HTTPException(status_code=400, detail="File too large (max 20MB)")

    tmp = tempfile.NamedTemporaryFile(suffix=suffix, delete=False)
    tmp_path = Path(tmp.name)
    try:
        tmp.write(content)
        tmp.close()

        engine = get_ocr()
        result = engine.ocr(str(tmp_path), cls=True)

        lines = []
        text_parts = []

        if result and result[0]:
            for line_data in result[0]:
                bbox = line_data[0]
                text = line_data[1][0]
                confidence = float(line_data[1][1])
                lines.append({
                    "text": text,
                    "confidence": round(confidence, 4),
                    "bbox": bbox,
                })
                text_parts.append(text)

        full_text = "\n".join(text_parts)
        logger.info("OCR processed %s: %d lines extracted", file.filename, len(lines))

        return {
            "text": full_text,
            "lines": lines,
            "line_count": len(lines),
        }
    except Exception as e:
        logger.error("OCR failed for %s: %s", file.filename, e)
        raise HTTPException(status_code=500, detail=f"OCR processing failed: {e}")
    finally:
        tmp_path.unlink(missing_ok=True)
