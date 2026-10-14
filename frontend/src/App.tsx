import { useEffect, useRef, useState } from 'react';
import { GetPhotoDataURL, LoadQuizPhotos, OpenQuiz } from '../wailsjs/go/main/App';
import { main } from '../wailsjs/go/models';
import { EventsOn } from '../wailsjs/runtime/runtime';
import './App.css';

type Species = {
    comName: string;
    sciName?: string;
    speciesCode?: string;
    frequency?: number;
    notes?: string;
    Habitat?: string;
};

type Quiz = {
    name: string;
    start_month: number;
    end_month: number;
    location: string;
    notes: string;
    species: Species[];
    taxonomyWarning?: string;
};

type PhotoResult = {
    photos: Record<string, string[]>;
    warnings: string[] | null;
};

type PhotoProgress = {
    current: number;
    total: number;
    species: string;
    stage: string;
};

const months = [
    '', 'January', 'February', 'March', 'April', 'May', 'June',
    'July', 'August', 'September', 'October', 'November', 'December',
];

function App() {
    const [quiz, setQuiz] = useState<Quiz | null>(null);
    const [photoResult, setPhotoResult] = useState<PhotoResult | null>(null);
    const [imageIndexes, setImageIndexes] = useState<Record<string, number>>({});
    const [answers, setAnswers] = useState<Record<string, string>>({});
    const [error, setError] = useState('');
    const [isOpening, setIsOpening] = useState(false);
    const [isFindingPhotos, setIsFindingPhotos] = useState(false);
    const [isPlaying, setIsPlaying] = useState(false);
    const [photoProgress, setPhotoProgress] = useState('');
    const [photoDataURLs, setPhotoDataURLs] = useState<Record<string, string>>({});
    const [photoLoadErrors, setPhotoLoadErrors] = useState<Record<string, string>>({});
    const requestedPhotoURLs = useRef(new Set<string>());

    useEffect(() => EventsOn('photo-search-progress', (progress: PhotoProgress) => {
        const position = progress.current > 0 ? `${progress.current}/${progress.total} ${progress.species}` : '';
        setPhotoProgress([position, progress.stage].filter(Boolean).join(' · '));
    }), []);

    useEffect(() => {
        if (!quiz || !photoResult || !isPlaying) return;
        for (const species of quiz.species) {
            const speciesKey = species.speciesCode || species.comName;
            const images = photoResult.photos[speciesKey] || [];
            const imageURL = images[imageIndexes[speciesKey] || 0];
            if (!imageURL || requestedPhotoURLs.current.has(imageURL)) continue;
            requestedPhotoURLs.current.add(imageURL);
            GetPhotoDataURL(imageURL)
                .then((dataURL) => setPhotoDataURLs((current) => ({ ...current, [imageURL]: dataURL })))
                .catch((caught) => setPhotoLoadErrors((current) => ({
                    ...current,
                    [imageURL]: caught instanceof Error ? caught.message : String(caught),
                })));
        }
    }, [quiz, photoResult, imageIndexes, isPlaying]);

    async function openQuiz() {
        setError('');
        setIsOpening(true);
        try {
            const loaded = await OpenQuiz() as Quiz | null;
            if (loaded) {
                setQuiz(loaded);
                setPhotoResult(null);
                setImageIndexes({});
                setAnswers({});
                setIsPlaying(false);
            }
        } catch (caught) {
            setError(caught instanceof Error ? caught.message : String(caught));
        } finally {
            setIsOpening(false);
        }
    }

    async function findPhotos() {
        if (!quiz) return;
        const missingCodes = quiz.species.filter((species) => !species.speciesCode).map((species) => species.comName);
        if (missingCodes.length > 0) {
            setError(quiz.taxonomyWarning || `eBird species codes are unavailable for: ${missingCodes.join(', ')}. Configure the taxonomy path, then reopen the quiz.`);
            return;
        }
        setError('');
        setIsFindingPhotos(true);
        setPhotoProgress('Starting photo search');
        try {
            const result = await LoadQuizPhotos(main.Quiz.createFrom(quiz)) as PhotoResult;
            setPhotoResult(result);
            setImageIndexes({});
            setAnswers({});
            if (Object.values(result.photos).some((images) => images.length > 0)) {
                setIsPlaying(true);
            }
        } catch (caught) {
            setError(caught instanceof Error ? caught.message : String(caught));
        } finally {
            setIsFindingPhotos(false);
            setPhotoProgress('');
        }
    }

    const period = quiz
        ? quiz.start_month === quiz.end_month
            ? months[quiz.start_month] || 'Not specified'
            : `${months[quiz.start_month] || '?'} to ${months[quiz.end_month] || '?'}`
        : '';
    const photoWarnings = photoResult?.warnings ?? [];

    return (
        <div className="app-shell">
            <header className="topbar">
                <div className="brand-lockup">
                    <span className="brand-mark" aria-hidden="true">P</span>
                    <span className="brand-name">PHOTO ID</span>
                </div>
                <span className="topbar-label">FIELD QUIZ</span>
                <button className="open-button" onClick={openQuiz} disabled={isOpening}>
                    {isOpening ? 'Opening...' : 'Open quiz'}
                </button>
            </header>

            <main className="main-content">
                {error && <div className="error-message" role="alert">{error}</div>}
                {!quiz ? (
                    <section className="empty-state">
                        <p className="eyebrow">FIELD LIBRARY <span>/</span> READY</p>
                        <div className="empty-layout">
                            <div>
                                <h1>Choose a quiz file</h1>
                                <p className="empty-copy">Open a JSON quiz to review its species, season, and region.</p>
                                <button className="primary-button" onClick={openQuiz} disabled={isOpening}>
                                    {isOpening ? 'Opening...' : 'Browse quiz files'}
                                </button>
                            </div>
                            <div className="field-stamp" aria-hidden="true">
                                <span>OBSERVATION</span><strong>01</strong><span>SPECIES SET</span>
                            </div>
                        </div>
                    </section>
                ) : isPlaying && photoResult ? (
                    <>
                        <div className="eyebrow">FIELD LIBRARY <span>/</span> PHOTO QUIZ</div>
                        <section className="quiz-heading">
                            <div>
                                <p className="section-label">REGION {quiz.location || 'NOT SET'} · {period.toUpperCase()}</p>
                                <h1>Identify the species</h1>
                            </div>
                            <div className="game-actions">
                                <span className="score-tag">
                                    {quiz.species.filter((species) => answers[species.speciesCode || species.comName] === species.comName).length} / {quiz.species.length} IDENTIFIED
                                </span>
                                <button className="secondary-button" onClick={() => setIsPlaying(false)}>Quiz details</button>
                            </div>
                        </section>
                        {photoWarnings.length > 0 && (
                            <div className="media-notice">{photoWarnings.join(' · ')}</div>
                        )}
                        <section className="game-grid" aria-label="Bird photo quiz">
                            {quiz.species.map((species, index) => {
                                const speciesKey = species.speciesCode || species.comName;
                                const images = photoResult.photos[speciesKey] || [];
                                const imageIndex = imageIndexes[speciesKey] || 0;
                                        const imageURL = images[imageIndex];
                                        const photoDataURL = imageURL ? photoDataURLs[imageURL] : '';
                                const answer = answers[speciesKey] || '';
                                const isCorrect = answer === species.comName;
                                const firstChoice = Math.max(0, Math.min(index - 3, quiz.species.length - 7));
                                const choices = quiz.species.slice(firstChoice, firstChoice + 7);

                                return (
                                    <article className="game-card" key={speciesKey}>
                                        <div className="photo-frame">
                                            {photoDataURL ? (
                                                <img src={photoDataURL} alt="Bird to identify" />
                                            ) : images.length > 0 ? (
                                                <div className="photo-placeholder">{imageURL && photoLoadErrors[imageURL] ? 'Photo unavailable' : 'Loading cached photo...'}</div>
                                            ) : (
                                                <div className="photo-placeholder">No photo available</div>
                                            )}
                                        </div>
                                        <div className="photo-controls">
                                            <button aria-label="Previous photo" disabled={images.length < 2} onClick={() => setImageIndexes((current) => ({ ...current, [speciesKey]: (imageIndex + images.length - 1) % images.length }))}>Previous</button>
                                            <span>{images.length > 0 ? `${imageIndex + 1} / ${images.length}` : 'NO IMAGES'}</span>
                                            <button aria-label="Next photo" disabled={images.length < 2} onClick={() => setImageIndexes((current) => ({ ...current, [speciesKey]: (imageIndex + 1) % images.length }))}>Next</button>
                                        </div>
                                        <div className="answer-area">
                                            <label htmlFor={`answer-${index}`}>Identify this bird</label>
                                            <select
                                                id={`answer-${index}`}
                                                className={answer ? (isCorrect ? 'answer-correct' : 'answer-incorrect') : ''}
                                                value={answer}
                                                onChange={(event) => setAnswers((current) => ({ ...current, [speciesKey]: event.target.value }))}
                                            >
                                                <option value="">Choose a species</option>
                                                {choices.map((choice) => (
                                                    <option key={choice.speciesCode || choice.comName} value={choice.comName}>{choice.comName}</option>
                                                ))}
                                            </select>
                                            {answer && <span className={`answer-feedback ${isCorrect ? 'correct' : 'incorrect'}`}>{isCorrect ? 'Correct' : 'Not quite'}</span>}
                                        </div>
                                    </article>
                                );
                            })}
                        </section>
                    </>
                ) : (
                    <>
                        <div className="eyebrow">FIELD LIBRARY <span>/</span> QUIZ PREVIEW</div>
                        <section className="quiz-heading">
                            <div>
                                <p className="section-label">REGION {quiz.location || 'NOT SET'}</p>
                                <h1>Quiz details for {quiz.name || 'Untitled quiz'}</h1>
                            </div>
                            <button className="primary-button" onClick={findPhotos} disabled={isFindingPhotos}>
                                {isFindingPhotos ? (photoProgress || 'Finding photos...') : 'Start Quiz'}
                            </button>
                        </section>
                        <section className="summary-strip" aria-label="Quiz details">
                            <div className="summary-item"><span className="summary-label">SEASON</span><strong>{period}</strong></div>
                            <div className="summary-item"><span className="summary-label">REGION CODE</span><strong>{quiz.location || 'Not specified'}</strong></div>
                            <div className="summary-item"><span className="summary-label">SPECIES</span><strong>{quiz.species.length}</strong></div>
                        </section>
                        {quiz.notes && <p className="quiz-notes">{quiz.notes}</p>}
                        <section className="species-section">
                            <div className="list-heading">
                                <div><p className="section-label">QUIZ CONTENT</p><h2>Species list</h2></div>
                                <span>{String(quiz.species.length).padStart(2, '0')} ENTRIES</span>
                            </div>
                            <div className="media-notice">
                                {photoResult
                                    ? photoWarnings.length > 0
                                        ? photoWarnings.join(' · ')
                                        : `${Object.values(photoResult.photos).flat().length} photos loaded.`
                                    : quiz.taxonomyWarning || `${quiz.species.filter((species) => species.speciesCode).length} eBird species codes resolved.`}
                            </div>
                            <ol className="species-list">
                                {quiz.species.map((species, index) => (
                                    <li className="species-row" key={`${species.comName}-${index}`}>
                                        <span className="species-index">{String(index + 1).padStart(2, '0')}</span>
                                        <div className="species-name">
                                            <strong>{species.comName}</strong>
                                            {species.sciName && <span>{species.sciName}</span>}
                                        </div>
                                        <div className="species-details">
                                            {species.frequency !== undefined && species.frequency > 0 && <span>{species.frequency}% frequency</span>}
                                            {species.Habitat && <span>{species.Habitat}</span>}
                                            {species.notes && <span>{species.notes}</span>}
                                        </div>
                                        <div className="species-link">
                                            {species.speciesCode
                                                ? <a href={`https://ebird.org/species/${species.speciesCode}`} target="_blank" rel="noreferrer">{species.speciesCode} · eBird</a>
                                                : <span>NO SPECIES CODE</span>}
                                        </div>
                                    </li>
                                ))}
                            </ol>
                        </section>
                    </>
                )}
            </main>
        </div>
    );
}

export default App;
