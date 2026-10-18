import { useEffect, useRef, useState } from 'react';
import { GetDestinationImages, GetPhotoDataURL, LoadQuizPhotos, OpenQuiz, OpenRecentQuiz } from '../wailsjs/go/main/App';
import { main } from '../wailsjs/go/models';
import { BrowserOpenURL, EventsOn } from '../wailsjs/runtime/runtime';
import grackleAvatar from './assets/images/grackle-avatar.jpg';
import './App.css';

type Species = {
    comName: string;
    sciName?: string;
    speciesCode?: string;
    frequency?: number;
    notes?: string;
    Habitat?: string;
    seenBefore?: boolean;
};

type Quiz = {
    name: string;
    path: string;
    start_month: number;
    end_month: number;
    location: string;
    notes: string;
    species: Species[];
    taxonomyWarning?: string;
};

type RecentQuiz = {
    name: string;
    path: string;
    lastRun: number;
    location?: string;
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

const recentQuizStorageKey = 'photo-id.recent-quizzes.v1';
const tripsStorageKey = 'photo-id.trips.v1';
const legacyTripStorageKey = 'photo-id.next-trip.v1';

type Trip = { id: string; place: string; date: string };

function newTripId(): string {
    return `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;
}

function isTripFields(item: unknown): item is { place: string; date: string } {
    if (typeof item !== 'object' || item === null) return false;
    const trip = item as Partial<Trip>;
    return typeof trip.place === 'string' && typeof trip.date === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(trip.date);
}

function readTrips(): Trip[] {
    try {
        const stored = window.localStorage.getItem(tripsStorageKey);
        if (stored !== null) {
            const parsed: unknown = JSON.parse(stored);
            if (!Array.isArray(parsed)) return [];
            return parsed.filter(isTripFields).map((trip) => ({ id: (trip as Partial<Trip>).id || newTripId(), place: trip.place, date: trip.date }));
        }
        const legacy: unknown = JSON.parse(window.localStorage.getItem(legacyTripStorageKey) || 'null');
        return isTripFields(legacy) ? [{ id: newTripId(), place: legacy.place, date: legacy.date }] : [];
    } catch {
        return [];
    }
}

function formatTripDate(date: string): string {
    const [year, month, day] = date.split('-').map(Number);
    return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: 'numeric' }).format(new Date(year, month - 1, day));
}

function daysUntil(date: string): number {
    const [year, month, day] = date.split('-').map(Number);
    const today = new Date();
    const todayUTC = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
    return Math.round((Date.UTC(year, month - 1, day) - todayUTC) / 86400000);
}
const maxRecentQuizzes = 10;

function readRecentQuizzes(): RecentQuiz[] {
    try {
        const stored: unknown = JSON.parse(window.localStorage.getItem(recentQuizStorageKey) || '[]');
        if (!Array.isArray(stored)) return [];
        return stored.filter((item): item is RecentQuiz => {
            if (typeof item !== 'object' || item === null) return false;
            const recent = item as Partial<RecentQuiz>;
            return typeof recent.name === 'string'
                && typeof recent.path === 'string'
                && typeof recent.lastRun === 'number'
                && Number.isFinite(recent.lastRun);
        }).slice(0, maxRecentQuizzes);
    } catch {
        return [];
    }
}

function formatRecentDate(timestamp: number): string {
    return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }).format(timestamp);
}

function App() {
    const [quiz, setQuiz] = useState<Quiz | null>(null);
    const [photoResult, setPhotoResult] = useState<PhotoResult | null>(null);
    const [imageIndexes, setImageIndexes] = useState<Record<string, number>>({});
    const [answers, setAnswers] = useState<Record<string, string>>({});
    const [fitWindow, setFitWindow] = useState<Record<string, boolean>>({});
    const [error, setError] = useState('');
    const [isOpening, setIsOpening] = useState(false);
    const [isFindingPhotos, setIsFindingPhotos] = useState(false);
    const [isPlaying, setIsPlaying] = useState(false);
    const [photoProgress, setPhotoProgress] = useState('');
    const [photoDataURLs, setPhotoDataURLs] = useState<Record<string, string>>({});
    const [photoLoadErrors, setPhotoLoadErrors] = useState<Record<string, string>>({});
    const [recentQuizzes, setRecentQuizzes] = useState<RecentQuiz[]>(readRecentQuizzes);
    const requestedPhotoURLs = useRef(new Set<string>());
    const [trips, setTrips] = useState<Trip[]>(readTrips);
    const [selectedTripId, setSelectedTripId] = useState('');
    const [editingTripId, setEditingTripId] = useState('');
    const [tripDraft, setTripDraft] = useState({ place: '', date: '' });
    const tripDialog = useRef<HTMLDialogElement>(null);
    const [destinationImages, setDestinationImages] = useState<main.DestinationImage[]>([]);
    const [destinationIndex, setDestinationIndex] = useState(0);
    const [destinationStatus, setDestinationStatus] = useState('');
    const sortedTrips = [...trips].sort((a, b) => a.date.localeCompare(b.date));
    const upcomingTrips = sortedTrips.filter((item) => daysUntil(item.date) >= 0);
    const trip = upcomingTrips.find((item) => item.id === selectedTripId) ?? upcomingTrips[0] ?? null;
    const otherTrips = upcomingTrips.filter((item) => item.id !== trip?.id);
    const tripPlace = trip?.place;

    useEffect(() => {
        try {
            window.localStorage.setItem(tripsStorageKey, JSON.stringify(trips));
        } catch {
            // Storage unavailable; trips last for this session only.
        }
    }, [trips]);

    function openDestinationArticle(pageUrl?: string) {
        if (pageUrl?.startsWith('https://en.wikipedia.org/wiki/')) BrowserOpenURL(pageUrl);
    }

    useEffect(() => {
        setDestinationImages([]);
        setDestinationIndex(0);
        setDestinationStatus('');
        if (!tripPlace) return;
        let cancelled = false;
        setDestinationStatus('Finding photos...');
        GetDestinationImages(tripPlace)
            .then((images) => {
                if (cancelled) return;
                setDestinationImages(images ?? []);
                setDestinationStatus(images?.length ? '' : `No Wikipedia photos found for "${tripPlace}". Check the spelling or try a more specific name.`);
            })
            .catch((error_) => {
                if (!cancelled) setDestinationStatus(`Photos unavailable: ${error_ instanceof Error ? error_.message : String(error_)}`);
            });
        return () => { cancelled = true; };
    }, [tripPlace]);

    useEffect(() => {
        if (destinationImages.length < 2) return;
        const timer = window.setInterval(() => setDestinationIndex((current) => (current + 1) % destinationImages.length), 6000);
        return () => window.clearInterval(timer);
    }, [destinationImages]);
    const photoSearchRun = useRef(0);

    useEffect(() => EventsOn('photo-search-progress', (progress: PhotoProgress) => {
        const position = progress.current > 0 ? `${progress.current}/${progress.total} ${progress.species}` : '';
        setPhotoProgress([position, progress.stage].filter(Boolean).join(' · '));
    }), []);

    useEffect(() => {
        try {
            window.localStorage.setItem(recentQuizStorageKey, JSON.stringify(recentQuizzes));
        } catch {
            return;
        }
    }, [recentQuizzes]);

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
                .catch((error_) => setPhotoLoadErrors((current) => ({
                    ...current,
                    [imageURL]: error_ instanceof Error ? error_.message : String(error_),
                })));
        }
    }, [quiz, photoResult, imageIndexes, isPlaying]);

    async function openQuiz() {
        setError('');
        setIsOpening(true);
        try {
            const loaded = await OpenQuiz() as Quiz | null;
            if (loaded) await startQuiz(loaded);
        } catch (error_) {
            setError(error_ instanceof Error ? error_.message : String(error_));
        } finally {
            setIsOpening(false);
        }
    }

    async function openRecentQuiz(path: string) {
        setError('');
        setIsOpening(true);
        try {
            const loaded = await OpenRecentQuiz(path) as Quiz;
            await startQuiz(loaded);
        } catch (error_) {
            setError(error_ instanceof Error ? error_.message : String(error_));
        } finally {
            setIsOpening(false);
        }
    }

    async function startQuiz(quizToStart: Quiz) {
        const searchRun = ++photoSearchRun.current;
        setQuiz(quizToStart);
        setPhotoResult({ photos: {}, warnings: [] });
        setImageIndexes({});
        setAnswers({});
        setError('');
        setIsPlaying(true);
        setIsFindingPhotos(true);

        const missingCodes = quizToStart.species.filter((species) => !species.speciesCode).map((species) => species.comName);
        if (missingCodes.length > 0) {
            const warning = quizToStart.taxonomyWarning || `eBird species codes are unavailable for: ${missingCodes.join(', ')}. Configure the taxonomy path, then reopen the quiz.`;
            setPhotoResult({ photos: {}, warnings: [warning] });
            setIsFindingPhotos(false);
            return;
        }
        setPhotoProgress('Starting photo search');
        try {
            const result = await LoadQuizPhotos(main.Quiz.createFrom(quizToStart)) as PhotoResult;
            if (photoSearchRun.current !== searchRun) return;
            setPhotoResult(result);
            setImageIndexes({});
            setAnswers({});
            const recentEntry = { name: quizToStart.name, path: quizToStart.path, lastRun: Date.now(), location: quizToStart.location };
            setRecentQuizzes((current) => [recentEntry, ...current.filter((item) => item.path !== recentEntry.path)].slice(0, maxRecentQuizzes));
        } catch (error_) {
            if (photoSearchRun.current !== searchRun) return;
            setError(error_ instanceof Error ? error_.message : String(error_));
            setPhotoResult({ photos: {}, warnings: [error_ instanceof Error ? error_.message : String(error_)] });
        } finally {
            if (photoSearchRun.current === searchRun) {
                setIsFindingPhotos(false);
                setPhotoProgress('');
            }
        }
    }

    function openTripDialog() {
        resetTripForm();
        tripDialog.current?.showModal();
    }

    function resetTripForm() {
        setEditingTripId('');
        setTripDraft({ place: '', date: '' });
    }

    function editTrip(item: Trip) {
        setEditingTripId(item.id);
        setTripDraft({ place: item.place, date: item.date });
    }

    function deleteTrip(id: string) {
        setTrips((current) => current.filter((item) => item.id !== id));
        if (editingTripId === id) resetTripForm();
    }

    function saveTrip(event: React.FormEvent) {
        event.preventDefault();
        const place = tripDraft.place.trim();
        if (!place || !tripDraft.date) return;
        if (editingTripId) {
            setTrips((current) => current.map((item) => (item.id === editingTripId ? { ...item, place, date: tripDraft.date } : item)));
        } else {
            setTrips((current) => [...current, { id: newTripId(), place, date: tripDraft.date }]);
        }
        resetTripForm();
    }

    const tripDays = trip ? daysUntil(trip.date) : null;

    function endQuiz() {
        photoSearchRun.current += 1;
        setQuiz(null);
        setPhotoResult(null);
        setImageIndexes({});
        setAnswers({});
        setError('');
        setIsPlaying(false);
        setIsFindingPhotos(false);
        setPhotoProgress('');
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
                    <span className="brand-mark">
                        <img src={grackleAvatar} alt="Grackle" />
                    </span>
                    <span className="brand-name">PHOTO ID</span>
                </div>
                <span className="topbar-label">FIELD QUIZ</span>
                {quiz && <button className="open-button" onClick={endQuiz}>End Quiz</button>}
            </header>

            <main className={quiz && isPlaying && photoResult ? 'main-content main-content-wide' : 'main-content'}>
                {error && <div className="error-message" role="alert">{error}</div>}
                {!quiz ? (
                    <section className="empty-state">
                        <p className="eyebrow">FIELD LIBRARY <span>/</span> READY</p>
                        <div className="empty-layout">
                            <div>
                                <h1>Choose a quiz file</h1>
                                <p className="empty-copy">Open a quiz to review expected bird species.</p>
                                <button className="primary-button" onClick={openQuiz} disabled={isOpening}>
                                    {isOpening ? 'Opening...' : 'Browse quiz files'}
                                </button>
                                {recentQuizzes.length > 0 && (
                                    <section className="recent-quizzes" aria-label="Recent quizzes">
                                        <div className="recent-quizzes-heading">
                                            <p className="section-label">RECENT QUIZZES</p>
                                            <span>LAST {recentQuizzes.length}</span>
                                        </div>
                                        <ol className="recent-quiz-list">
                                            {recentQuizzes.map((recent) => (
                                                <li key={recent.path}>
                                                    <button className="recent-quiz-button" onClick={() => openRecentQuiz(recent.path)} disabled={isOpening || isFindingPhotos}>
                                                        <span className="recent-quiz-region">{recent.location || ''}</span>
                                                        <strong>{recent.name}</strong>
                                                        <time dateTime={new Date(recent.lastRun).toISOString()}>{formatRecentDate(recent.lastRun)}</time>
                                                    </button>
                                                </li>
                                            ))}
                                        </ol>
                                    </section>
                                )}
                            </div>
                            <div className="trip-panel">
                            <button className="field-stamp" onClick={openTripDialog} aria-label={trip ? `Next trip: ${trip.place}. Click to manage trips.` : 'Add a trip'}>
                                {trip && tripDays !== null ? (
                                    tripDays > 0 ? (
                                        <><span>COUNTDOWN</span><strong>{tripDays}</strong><span>{tripDays === 1 ? 'DAY' : 'DAYS'} UNTIL {trip.place.toUpperCase()}</span></>
                                    ) : (
                                        <><span>TODAY</span><strong>Go!</strong><span>{trip.place.toUpperCase()}</span></>
                                    )
                                ) : (
                                    <><span>NEXT TRIP</span><strong>?</strong><span>CLICK TO SET</span></>
                                )}
                            </button>
                            {otherTrips.length > 0 && (
                                <section className="other-trips" aria-label="Other upcoming trips">
                                    <p className="section-label">ALSO COMING UP</p>
                                    <ul>
                                        {otherTrips.map((item) => (
                                            <li key={item.id}>
                                                <button onClick={() => setSelectedTripId(item.id)} title="Show this trip">
                                                    <span>{item.place}</span>
                                                    <span>{formatTripDate(item.date)}</span>
                                                    <span>{daysUntil(item.date)} {daysUntil(item.date) === 1 ? 'day' : 'days'}</span>
                                                </button>
                                            </li>
                                        ))}
                                    </ul>
                                </section>
                            )}
                            {destinationStatus && <p className="destination-status" role="status">{destinationStatus}</p>}
                            {destinationImages.length > 0 && (
                                <figure className="destination-figure">
                                    <button
                                        className="destination-gallery"
                                        onClick={() => openDestinationArticle(destinationImages[destinationIndex]?.pageUrl)}
                                        aria-label={`Open the Wikipedia article for ${tripPlace}`}
                                        title="Open on Wikipedia"
                                    >
                                        {destinationImages.map((image, index) => (
                                            <img key={image.dataUrl.slice(-64)} src={image.dataUrl} alt={image.caption} className={index === destinationIndex ? 'active' : ''} />
                                        ))}
                                    </button>
                                    <figcaption>{destinationImages[destinationIndex]?.caption}</figcaption>
                                </figure>
                            )}
                            </div>
                        </div>
                    </section>
                ) : isPlaying && photoResult ? (
                    <>
                        <div className="eyebrow">FIELD LIBRARY <span>/</span> PHOTO QUIZ</div>
                        <section className="quiz-heading">
                            <div>
                                <p className="section-label">REGION {quiz.location || 'NOT SET'} · {period.toUpperCase()}</p>
                                <h1>Quiz: {quiz.name || 'Untitled quiz'}</h1>
                            </div>
                            <div className="game-actions">
                                <span className="score-tag">
                                    {quiz.species.filter((species) => answers[species.speciesCode || species.comName] === species.comName).length} / {quiz.species.length} IDENTIFIED
                                </span>
                                <button className="secondary-button" onClick={() => setIsPlaying(false)}>Quiz details</button>
                            </div>
                        </section>
                        {quiz.notes && <p className="quiz-notes game-notes">{quiz.notes}</p>}
                        {isFindingPhotos && (
                            <div className="media-notice" role="status">{photoProgress || 'Finding photos...'}</div>
                        )}
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
                                const isFitWindow = !!fitWindow[speciesKey];

                                return (
                                    <article className={isFitWindow ? 'game-card game-card-fit-window' : 'game-card'} key={speciesKey}>
                                        <div className="photo-frame">
                                            {photoDataURL ? (
                                                <img src={photoDataURL} alt="Bird to identify" />
                                            ) : images.length > 0 ? (
                                                <div className="photo-placeholder">{imageURL && photoLoadErrors[imageURL] ? 'Photo unavailable' : 'Loading cached photo...'}</div>
                                            ) : (
                                                <div className="photo-placeholder">No photo available</div>
                                            )}
                                        </div>
                                        {photoDataURL && (
                                            <button className="fit-toggle" aria-pressed={isFitWindow} onClick={() => setFitWindow((current) => ({ ...current, [speciesKey]: !isFitWindow }))}>
                                                {isFitWindow ? 'Fit to width' : 'Full Size'}
                                            </button>
                                        )}
                                        <div className="photo-controls">
                                            <button aria-label="Previous photo" disabled={images.length < 2} onClick={() => setImageIndexes((current) => ({ ...current, [speciesKey]: (imageIndex + images.length - 1) % images.length }))}>Previous</button>
                                            <span>{images.length > 0 ? `${imageIndex + 1} / ${images.length}` : 'NO IMAGES'}</span>
                                            <button aria-label="Next photo" disabled={images.length < 2} onClick={() => setImageIndexes((current) => ({ ...current, [speciesKey]: (imageIndex + 1) % images.length }))}>Next</button>
                                        </div>
                                        <div className="answer-area">
                                            <label htmlFor={`answer-${index}`}>
                                                <span className="answer-prompt">
                                                    {species.seenBefore && <span className="seen-dot" role="img" aria-label="Seen before" title="Seen before" />}
                                                    Identify this bird
                                                </span>
                                                {typeof species.frequency === 'number' && Number.isFinite(species.frequency) && (
                                                    <span className="answer-frequency">Frequency: {species.frequency.toFixed(2)}%</span>
                                                )}
                                            </label>
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
                                <p className="section-label">REGION {quiz.location || 'NOT SET'} · {period.toUpperCase()}</p>
                                <h1>Quiz details for {quiz.name || 'Untitled quiz'}</h1>
                            </div>
                            <button className="primary-button" onClick={() => (photoResult && !isFindingPhotos ? setIsPlaying(true) : startQuiz(quiz))} disabled={isFindingPhotos}>
                                {isFindingPhotos ? (photoProgress || 'Finding photos...') : 'Return to Quiz'}
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
                                            <div className="species-name-primary">
                                                {species.seenBefore && <span className="seen-dot" role="img" aria-label="Seen before" title="Seen before" />}
                                                <strong>{species.comName}</strong>
                                            </div>
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
            <dialog ref={tripDialog} className="trip-dialog" aria-labelledby="trip-dialog-title">
                <form onSubmit={saveTrip}>
                    <h2 id="trip-dialog-title">Trips</h2>
                    {sortedTrips.length > 0 && (
                        <ul className="trip-manage-list">
                            {sortedTrips.map((item) => (
                                <li key={item.id}>
                                    <span>{item.place}</span>
                                    <span>{formatTripDate(item.date)}{daysUntil(item.date) < 0 ? ' (passed)' : ''}</span>
                                    <button type="button" className="secondary-button" onClick={() => editTrip(item)}>Edit</button>
                                    <button type="button" className="secondary-button" onClick={() => deleteTrip(item.id)}>Delete</button>
                                </li>
                            ))}
                        </ul>
                    )}
                    <p className="section-label">{editingTripId ? 'EDIT TRIP' : 'ADD A TRIP'}</p>
                    <label>Where<input type="text" value={tripDraft.place} onChange={(event) => setTripDraft((current) => ({ ...current, place: event.target.value }))} placeholder="Argentina" required /></label>
                    <label>When<input type="date" value={tripDraft.date} onChange={(event) => setTripDraft((current) => ({ ...current, date: event.target.value }))} required /></label>
                    <div className="trip-dialog-actions">
                        {editingTripId && <button type="button" className="secondary-button" onClick={resetTripForm}>Cancel edit</button>}
                        <button type="submit" className="primary-button">{editingTripId ? 'Save' : 'Add trip'}</button>
                        <button type="button" className="secondary-button" onClick={() => tripDialog.current?.close()}>Done</button>
                    </div>
                </form>
            </dialog>
        </div>
    );
}

export default App;
