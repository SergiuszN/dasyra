-- PDFs and text files sent in Telegram, downloaded only when read_document asks for them
ALTER TABLE conversations ADD COLUMN document_file_id TEXT;
