"""PaddleOCR microservice — extracts text from receipt images."""

import logging
import tempfile
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI, File, HTTPException, UploadFile
from paddleocr import PaddleOCR
from PIL import Image

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

_ocr_engine: PaddleOCR | None = None

ALLOWED_EXTENSIONS = {".jpg", ".jpeg", ".png", ".bmp", ".tiff", ".tif", ".webp"}
MAX_FILE_SIZE = 20 * 1024 * 1024  # 20 MB
MAX_IMAGE_DIM = 1600  # Resize images larger than this to prevent OOM


def get_ocr() -> PaddleOCR:
    global _ocr_engine
    if _ocr_engine is None:
        logger.info("Initializing PaddleOCR engine (PP-OCRv4, CPU, en)...")
        _ocr_engine = PaddleOCR(
            ocr_version="PP-OCRv4",
            lang="en",
            use_angle_cls=True,
            use_gpu=False,
            rec_batch_num=2,  # Lower batch size to reduce memory usage
        )
        logger.info("PaddleOCR engine ready.")
    return _ocr_engine


def resize_image(img_path: Path) -> Path:
    """Resize image if too large, to prevent OOM during OCR."""
    with Image.open(img_path) as img:
        w, h = img.size
        if max(w, h) <= MAX_IMAGE_DIM:
            return img_path

        ratio = MAX_IMAGE_DIM / max(w, h)
        new_w, new_h = int(w * ratio), int(h * ratio)
        logger.info("Resizing image from %dx%d to %dx%d", w, h, new_w, new_h)
        resized = img.resize((new_w, new_h), Image.LANCZOS)

        resized_path = img_path.with_suffix(".resized.jpg")
        resized.save(resized_path, "JPEG", quality=90)
        return resized_path


@asynccontextmanager
async def lifespan(app: FastAPI):
    get_ocr()
    yield


app = FastAPI(title="OCR Service", version="1.0.0", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok", "engine": "PP-OCRv4"}


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
    resized_path = None
    try:
        tmp.write(content)
        tmp.close()

        # Resize large images to prevent OOM.
        ocr_path = resize_image(tmp_path)
        if ocr_path != tmp_path:
            resized_path = ocr_path

        engine = get_ocr()
        result = engine.ocr(str(ocr_path))

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
        if resized_path:
            resized_path.unlink(missing_ok=True)
